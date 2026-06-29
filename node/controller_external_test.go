package node

import (
	"fmt"
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
	t.Setenv("V2NODE_EXTERNAL_CONFIG_DIR", t.TempDir())
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
