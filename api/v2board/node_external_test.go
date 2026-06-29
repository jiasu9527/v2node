package panel

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-resty/resty/v2"
)

func TestGetNodeInfoAcceptsExternalProtocols(t *testing.T) {
	for _, protocol := range []string{"juicity", "mieru"} {
		t.Run(protocol, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v2/server/config" {
					t.Fatalf("unexpected path %q", r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprintf(w, `{"protocol":%q,"server_port":443,"external_protocol":true,"traffic_mode":"unsupported","password_mode":"uuid","base_config":{"push_interval":60,"pull_interval":60}}`, protocol)
			}))
			defer srv.Close()

			client := &Client{
				client:  resty.New().SetBaseURL(srv.URL),
				APIHost: srv.URL,
				NodeId:  7,
			}

			node, err := client.GetNodeInfo(context.Background())
			if err != nil {
				t.Fatalf("GetNodeInfo() error = %v", err)
			}
			if node.Type != protocol || node.Security != None {
				t.Fatalf("unexpected node info: type=%q security=%d", node.Type, node.Security)
			}
			if node.Common == nil || !node.Common.ExternalProtocol || node.Common.TrafficMode != "unsupported" || node.Common.PasswordMode != "uuid" {
				t.Fatalf("external protocol fields were not parsed: %#v", node.Common)
			}
		})
	}
}
