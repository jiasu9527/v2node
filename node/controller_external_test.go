package node

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	panel "github.com/wyx2685/v2node/api/v2board"
	"github.com/wyx2685/v2node/conf"
	vcore "github.com/wyx2685/v2node/core"
	"github.com/wyx2685/v2node/limiter"
)

func TestControllerStartExternalProtocolSkipsXrayInbound(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
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
		Common:       &panel.CommonNode{Protocol: "juicity", ExternalProtocol: true, TrafficMode: "unsupported", PasswordMode: "uuid", ServerPort: 0, TlsSettings: panel.TlsSettings{CertFile: "testdata/missing.crt", KeyFile: "testdata/missing.key"}, BaseConfig: &panel.BaseConfig{}},
		PushInterval: 60 * time.Second,
		PullInterval: 60 * time.Second,
	}
	controller := NewController(api, &conf.NodeConfig{NodeID: 9}, info)
	core := &vcore.V2Core{ReloadCh: make(chan struct{}, 1)}

	if err := controller.Start(core); err == nil {
		_ = controller.Close()
		t.Fatalf("Start() succeeded with missing juicity cert; want embedded server validation error")
	}
}

func TestControllerStartMieruUsesEmbeddedServerWithoutExternalBinary(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	port := unusedTCPPort(t)
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
		Type:         "mieru",
		Security:     panel.None,
		Tag:          fmt.Sprintf("[%s]-mieru:9", srv.URL),
		Common:       &panel.CommonNode{Protocol: "mieru", ExternalProtocol: true, TrafficMode: "metrics", PasswordMode: "uuid", ServerPort: port, Transport: "TCP", BaseConfig: &panel.BaseConfig{}},
		PushInterval: time.Hour,
		PullInterval: time.Hour,
	}
	controller := NewController(api, &conf.NodeConfig{NodeID: 9}, info)
	core := &vcore.V2Core{ReloadCh: make(chan struct{}, 1)}

	if err := controller.Start(core); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer controller.Close()
	if controller.embeddedProtocolServer == nil || controller.embeddedProtocolServer.Protocol() != "mieru" {
		t.Fatalf("embedded protocol server not started: %#v", controller.embeddedProtocolServer)
	}
}

func TestControllerReloadExternalProtocolUsesEmbeddedServer(t *testing.T) {
	port := unusedTCPPort(t)
	info := &panel.NodeInfo{
		Id:       10,
		Type:     "mieru",
		Tag:      "embedded-mieru:10",
		Common:   &panel.CommonNode{Protocol: "mieru", ExternalProtocol: true, TrafficMode: "metrics", PasswordMode: "uuid", ServerPort: port, Transport: "TCP", BaseConfig: &panel.BaseConfig{}},
		Security: panel.None,
	}
	controller := &Controller{tag: info.Tag, info: info, userList: []panel.UserInfo{{Id: 1, Uuid: "11111111-1111-1111-1111-111111111111"}}}
	if err := controller.reloadExternalProtocol(info, controller.userList); err != nil {
		t.Fatalf("reloadExternalProtocol() error = %v", err)
	}
	defer controller.Close()
	if controller.embeddedProtocolServer == nil || controller.embeddedProtocolServer.Protocol() != "mieru" {
		t.Fatalf("embedded protocol server not started: %#v", controller.embeddedProtocolServer)
	}
}

func unusedTCPPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen unused port: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}
