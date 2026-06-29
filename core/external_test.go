package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	panel "github.com/wyx2685/v2node/api/v2board"
)

func TestRenderJuicityConfig(t *testing.T) {
	node := &panel.NodeInfo{
		Id:   7,
		Type: "juicity",
		Common: &panel.CommonNode{
			Protocol:          "juicity",
			ServerPort:        443,
			CongestionControl: "bbr",
			TlsSettings: panel.TlsSettings{
				CertFile: "/etc/v2node/juicity7.cer",
				KeyFile:  "/etc/v2node/juicity7.key",
			},
		},
	}
	users := []panel.UserInfo{{Id: 1, Uuid: "user-uuid-1"}, {Id: 2, Uuid: "user-uuid-2"}}

	raw, err := RenderJuicityConfig(node, users)
	if err != nil {
		t.Fatalf("RenderJuicityConfig() error = %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if cfg["listen"] != ":443" || cfg["certificate"] != "/etc/v2node/juicity7.cer" || cfg["private_key"] != "/etc/v2node/juicity7.key" || cfg["congestion_control"] != "bbr" {
		t.Fatalf("unexpected juicity config: %#v", cfg)
	}
	usersMap, ok := cfg["users"].(map[string]any)
	if !ok || usersMap["user-uuid-1"] != "user-uuid-1" || usersMap["user-uuid-2"] != "user-uuid-2" {
		t.Fatalf("unexpected juicity users: %#v", cfg["users"])
	}
}

func TestRenderMieruConfig(t *testing.T) {
	node := &panel.NodeInfo{
		Id:   8,
		Type: "mieru",
		Common: &panel.CommonNode{
			Protocol:         "mieru",
			ServerPort:       2999,
			Transport:        "UDP",
			MTU:              1280,
			Multiplexing:     "MULTIPLEXING_LOW",
			PasswordMode:     "uuid",
			ExternalProtocol: true,
		},
	}
	users := []panel.UserInfo{{Id: 1, Uuid: "user-uuid-1"}}

	raw, err := RenderMieruConfig(node, users)
	if err != nil {
		t.Fatalf("RenderMieruConfig() error = %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	bindings, ok := cfg["portBindings"].([]any)
	if !ok || len(bindings) != 1 {
		t.Fatalf("expected one port binding, got %#v", cfg["portBindings"])
	}
	binding := bindings[0].(map[string]any)
	if binding["port"] != float64(2999) || binding["protocol"] != "UDP" {
		t.Fatalf("unexpected mieru binding: %#v", binding)
	}
	if cfg["mtu"] != float64(1280) || cfg["loggingLevel"] != "INFO" {
		t.Fatalf("unexpected mieru top-level settings: %#v", cfg)
	}
	usersList, ok := cfg["users"].([]any)
	if !ok || len(usersList) != 1 {
		t.Fatalf("expected one mieru user, got %#v", cfg["users"])
	}
	user := usersList[0].(map[string]any)
	if user["name"] != "1" || user["password"] != "user-uuid-1" {
		t.Fatalf("unexpected mieru user: %#v", user)
	}
}

func TestMieruExternalProcessUsesApplyConfigAndServiceCommands(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("V2NODE_EXTERNAL_CONFIG_DIR", tmp)
	calls := filepath.Join(tmp, "calls")
	bin := filepath.Join(tmp, "mita")
	script := fmt.Sprintf("#!/usr/bin/env sh\necho \"$@\" >> %q\n", calls)
	if err := os.WriteFile(bin, []byte(script), 0755); err != nil {
		t.Fatalf("write fake mita: %v", err)
	}
	t.Setenv("PATH", tmp+string(os.PathListSeparator)+os.Getenv("PATH"))

	node := &panel.NodeInfo{Id: 8, Type: "mieru", Common: &panel.CommonNode{Protocol: "mieru", ExternalProtocol: true, ServerPort: 2999}}
	process, err := NewExternalProcess(node, []panel.UserInfo{{Id: 1, Uuid: "user-uuid"}})
	if err != nil {
		t.Fatalf("NewExternalProcess() error = %v", err)
	}
	if err := process.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := process.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	raw, err := os.ReadFile(calls)
	if err != nil {
		t.Fatalf("read calls: %v", err)
	}
	body := string(raw)
	if !strings.Contains(body, "apply config "+process.ConfigPath) || !strings.Contains(body, "start") || !strings.Contains(body, "stop") {
		t.Fatalf("unexpected mita calls: %q", body)
	}
}

func TestJuicityExternalProcessUsesRunConfigCommand(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("V2NODE_EXTERNAL_CONFIG_DIR", tmp)
	calls := filepath.Join(tmp, "calls")
	bin := filepath.Join(tmp, "juicity-server")
	script := fmt.Sprintf("#!/usr/bin/env sh\necho \"$@\" > %q\ntrap 'exit 0' TERM INT\nwhile true; do sleep 1; done\n", calls)
	if err := os.WriteFile(bin, []byte(script), 0755); err != nil {
		t.Fatalf("write fake juicity-server: %v", err)
	}
	t.Setenv("PATH", tmp+string(os.PathListSeparator)+os.Getenv("PATH"))

	node := &panel.NodeInfo{Id: 7, Type: "juicity", Common: &panel.CommonNode{Protocol: "juicity", ExternalProtocol: true, ServerPort: 443}}
	process, err := NewExternalProcess(node, []panel.UserInfo{{Id: 1, Uuid: "user-uuid"}})
	if err != nil {
		t.Fatalf("NewExternalProcess() error = %v", err)
	}
	if err := process.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer process.Stop()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if raw, err := os.ReadFile(calls); err == nil {
			body := string(raw)
			if strings.Contains(body, "run -c "+process.ConfigPath) {
				return
			}
			t.Fatalf("unexpected juicity call: %q", body)
		}
		if time.Now().After(deadline) {
			t.Fatalf("juicity call was not recorded")
		}
		time.Sleep(20 * time.Millisecond)
	}
}
