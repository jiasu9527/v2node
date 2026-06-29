package node

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	panel "github.com/wyx2685/v2node/api/v2board"
	"github.com/wyx2685/v2node/conf"
	vcore "github.com/wyx2685/v2node/core"
	"github.com/wyx2685/v2node/limiter"
)

func installFakeExternalBinary(t *testing.T, dir string, name string, marker string) {
	t.Helper()
	bin := filepath.Join(dir, name)
	script := fmt.Sprintf(`#!/usr/bin/env sh
if [ -n %q ]; then echo "$@" > %q; fi
trap 'exit 0' TERM INT
while true; do sleep 1; done
`, marker, marker)
	if err := os.WriteFile(bin, []byte(script), 0755); err != nil {
		t.Fatalf("write fake %s: %v", name, err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestControllerStartExternalProtocolSkipsXrayInbound(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("V2NODE_EXTERNAL_CONFIG_DIR", tmp)
	installFakeExternalBinary(t, tmp, "juicity-server", "")
	limiter.Init()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/server/UniProxy/user":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"users":[{"id":1,"uuid":"11111111-1111-1111-1111-111111111111"}]}`))
		case "/api/v1/server/UniProxy/alivelist":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"alive":{}}`))
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer srv.Close()

	api, err := panel.New(&conf.NodeConfig{APIHost: srv.URL, NodeID: 9, Key: "secret"})
	if err != nil {
		t.Fatalf("panel.New() error = %v", err)
	}
	info := &panel.NodeInfo{
		Id:           9,
		Type:         "juicity",
		Security:     panel.None,
		Tag:          fmt.Sprintf("[%s]-juicity:9", srv.URL),
		Common:       &panel.CommonNode{Protocol: "juicity", ExternalProtocol: true, TrafficMode: "unsupported", PasswordMode: "uuid", BaseConfig: &panel.BaseConfig{}},
		PushInterval: 60 * time.Second,
		PullInterval: 60 * time.Second,
	}
	controller := NewController(api, &conf.NodeConfig{NodeID: 9}, info)
	core := &vcore.V2Core{ReloadCh: make(chan struct{}, 1)}

	if err := controller.Start(core); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer controller.Close()
	if controller.info == nil || controller.info.Type != "juicity" {
		t.Fatalf("unexpected controller info: %#v", controller.info)
	}
}

func TestControllerStartExternalProtocolStartsAndStopsProcess(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("V2NODE_EXTERNAL_CONFIG_DIR", tmp)
	marker := filepath.Join(tmp, "started")
	installFakeExternalBinary(t, tmp, "juicity-server", marker)
	limiter.Init()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/server/UniProxy/user":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"users":[{"id":1,"uuid":"11111111-1111-1111-1111-111111111111"}]}`))
		case "/api/v1/server/UniProxy/alivelist":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"alive":{}}`))
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer srv.Close()

	api, err := panel.New(&conf.NodeConfig{APIHost: srv.URL, NodeID: 9, Key: "secret"})
	if err != nil {
		t.Fatalf("panel.New() error = %v", err)
	}
	info := &panel.NodeInfo{
		Id:           9,
		Type:         "juicity",
		Security:     panel.None,
		Tag:          fmt.Sprintf("[%s]-juicity:9", srv.URL),
		Common:       &panel.CommonNode{Protocol: "juicity", ExternalProtocol: true, TrafficMode: "unsupported", PasswordMode: "uuid", ServerPort: 443, BaseConfig: &panel.BaseConfig{}},
		PushInterval: time.Hour,
		PullInterval: time.Hour,
	}
	controller := NewController(api, &conf.NodeConfig{NodeID: 9}, info)
	core := &vcore.V2Core{ReloadCh: make(chan struct{}, 1)}

	if err := controller.Start(core); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		if time.Now().After(deadline) {
			_ = controller.Close()
			t.Fatalf("external process was not started: marker %s was not created", marker)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := controller.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestControllerReloadExternalProtocolRerendersConfig(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("V2NODE_EXTERNAL_CONFIG_DIR", tmp)
	installFakeExternalBinary(t, tmp, "juicity-server", "")
	info := &panel.NodeInfo{
		Id:       10,
		Type:     "juicity",
		Tag:      "external-juicity:10",
		Common:   &panel.CommonNode{Protocol: "juicity", ExternalProtocol: true, TrafficMode: "unsupported", PasswordMode: "uuid", ServerPort: 443, BaseConfig: &panel.BaseConfig{}},
		Security: panel.None,
	}
	controller := &Controller{tag: info.Tag, info: info, userList: []panel.UserInfo{{Id: 1, Uuid: "old-user"}}}
	if err := controller.reloadExternalProtocol(info, controller.userList); err != nil {
		t.Fatalf("initial reloadExternalProtocol() error = %v", err)
	}
	defer controller.Close()

	newUsers := []panel.UserInfo{{Id: 2, Uuid: "new-user"}}
	if err := controller.reloadExternalProtocol(info, newUsers); err != nil {
		t.Fatalf("reloadExternalProtocol() error = %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(tmp, "external-juicity-10.json"))
	if err != nil {
		t.Fatalf("read external config: %v", err)
	}
	body := string(raw)
	if !strings.Contains(body, "new-user") || strings.Contains(body, "old-user") {
		t.Fatalf("external config was not rerendered for new users: %s", body)
	}
}
