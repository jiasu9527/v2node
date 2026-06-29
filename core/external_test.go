package core

import (
	"encoding/json"
	"testing"

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
	if binding["port"] != float64(2999) || binding["protocol"] != "UDP" || binding["mtu"] != float64(1280) || binding["multiplexing"] != "MULTIPLEXING_LOW" {
		t.Fatalf("unexpected mieru binding: %#v", binding)
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
