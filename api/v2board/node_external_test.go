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
	for _, protocol := range []string{"juicity", "mieru", "naive"} {
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
			if node.Common.ObservationAudit != nil {
				t.Fatalf("external protocol falsely enabled observation: %#v", node.Common.ObservationAudit)
			}
		})
	}
}

func TestGetNodeInfoParsesDedicatedObservationAllowlist(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/server/config" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if r.Header.Get("X-Observation-Capability") != "auto-v2" {
			t.Errorf("missing all-user capability header: %q", r.Header.Get("X-Observation-Capability"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"protocol":"anytls","server_port":443,"base_config":{"push_interval":60,"pull_interval":60},"observation_audit":{"enable":true,"allowed_user_ids":[10,20],"report_interval":45}}`))
	}))
	defer srv.Close()
	client := &Client{client: resty.New().SetBaseURL(srv.URL), APIHost: srv.URL, NodeId: 7}
	info, err := client.GetNodeInfo(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.Common.ObservationAudit == nil || !info.Common.ObservationAudit.Enable || len(info.Common.ObservationAudit.AllowedUserIDs) != 2 || info.Common.ObservationAudit.ReportIntervalDuration().Seconds() != 45 {
		t.Fatalf("observation config not parsed: %#v", info.Common.ObservationAudit)
	}
}

func TestGetNodeInfoParsesAllUserObservation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/server/config" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"protocol":"anytls","server_port":443,"base_config":{"push_interval":60,"pull_interval":60},"observation_audit":{"enable":true,"mode":"all","report_interval":45}}`))
	}))
	defer srv.Close()
	client := &Client{client: resty.New().SetBaseURL(srv.URL), APIHost: srv.URL, NodeId: 7}
	info, err := client.GetNodeInfo(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.Common.ObservationAudit == nil || !info.Common.ObservationAudit.Enable || info.Common.ObservationAudit.Mode != "all" {
		t.Fatalf("all-user observation config not parsed: %#v", info.Common.ObservationAudit)
	}
}

func TestGetNodeInfoDefaultsToAllUserObservation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		config  string
		enabled bool
	}{
		{name: "omitted", config: "", enabled: true},
		{name: "explicitly disabled", config: `,"observation_audit":{"enable":false,"mode":"all"}`, enabled: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got := r.Header.Get("X-Observation-Capability"); got != "auto-v2" {
					t.Errorf("capability = %q, want auto-v2", got)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprintf(w, `{"protocol":"anytls","server_port":443,"base_config":{"push_interval":60,"pull_interval":60}%s}`, tc.config)
			}))
			defer srv.Close()
			client := &Client{client: resty.New().SetBaseURL(srv.URL), APIHost: srv.URL, NodeId: 7}
			info, err := client.GetNodeInfo(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if audit := info.Common.ObservationAudit; audit == nil || audit.Enable != tc.enabled || audit.Mode != "all" {
				t.Fatalf("audit = %#v, want enabled=%t and all-user mode", audit, tc.enabled)
			}
		})
	}
}
