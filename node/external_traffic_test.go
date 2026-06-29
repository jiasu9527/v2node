package node

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	panel "github.com/wyx2685/v2node/api/v2board"
	"github.com/wyx2685/v2node/conf"
	"github.com/wyx2685/v2node/core"
)

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func TestReportUserTrafficTaskUsesMieruExternalMetrics(t *testing.T) {
	tmp := t.TempDir()
	metricsPath := filepath.Join(tmp, "metrics.json")
	bin := filepath.Join(tmp, "mita")
	script := "#!/usr/bin/env sh\n" +
		"if [ \"$1 $2\" = \"get metrics\" ]; then cat " + shellQuote(metricsPath) + "; exit 0; fi\n" +
		"exit 0\n"
	if err := os.WriteFile(bin, []byte(script), 0755); err != nil {
		t.Fatalf("write fake mita: %v", err)
	}
	t.Setenv("PATH", tmp+string(os.PathListSeparator)+os.Getenv("PATH"))

	pushes := make([]map[int][]int64, 0)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/server/UniProxy/push" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		var body map[int][]int64
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode push body: %v", err)
		}
		pushes = append(pushes, body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	api, err := panel.New(&conf.NodeConfig{APIHost: srv.URL, NodeID: 8, Key: "secret"})
	if err != nil {
		t.Fatalf("panel.New() error = %v", err)
	}
	info := &panel.NodeInfo{
		Id:   8,
		Type: "mieru",
		Tag:  "external-mieru:8",
		Common: &panel.CommonNode{Protocol: "mieru", ExternalProtocol: true, TrafficMode: "metrics", BaseConfig: &panel.BaseConfig{
			NodeReportMinTraffic: 0,
		}},
	}
	controller := &Controller{
		apiClient:                api,
		info:                     info,
		tag:                      info.Tag,
		userList:                 []panel.UserInfo{{Id: 1001, Uuid: "uuid-1001"}},
		externalTrafficCollector: core.NewExternalTrafficCollector(info),
	}

	if err := os.WriteFile(metricsPath, []byte(`{"users":{"1001":{"DownloadBytes":5000,"UploadBytes":7000}}}`), 0644); err != nil {
		t.Fatalf("write first metrics: %v", err)
	}
	if err := controller.reportUserTrafficTask(context.Background()); err != nil {
		t.Fatalf("first reportUserTrafficTask() error = %v", err)
	}
	if len(pushes) != 0 {
		t.Fatalf("first report should only establish baseline, got pushes %#v", pushes)
	}

	if err := os.WriteFile(metricsPath, []byte(`{"users":{"1001":{"DownloadBytes":9000,"UploadBytes":9000}}}`), 0644); err != nil {
		t.Fatalf("write second metrics: %v", err)
	}
	if err := controller.reportUserTrafficTask(context.Background()); err != nil {
		t.Fatalf("second reportUserTrafficTask() error = %v", err)
	}
	if len(pushes) != 1 {
		t.Fatalf("push count = %d, want 1", len(pushes))
	}
	got := pushes[0][1001]
	if len(got) != 2 || got[0] != 2000 || got[1] != 4000 {
		t.Fatalf("push body for uid 1001 = %#v, want [2000 4000]", got)
	}
}

func TestReportOnlineUsersTaskUsesMieruExternalMetrics(t *testing.T) {
	tmp := t.TempDir()
	metricsPath := filepath.Join(tmp, "metrics.json")
	bin := filepath.Join(tmp, "mita")
	script := "#!/usr/bin/env sh\n" +
		"if [ \"$1 $2\" = \"get metrics\" ]; then cat " + shellQuote(metricsPath) + "; exit 0; fi\n" +
		"exit 0\n"
	if err := os.WriteFile(bin, []byte(script), 0755); err != nil {
		t.Fatalf("write fake mita: %v", err)
	}
	t.Setenv("PATH", tmp+string(os.PathListSeparator)+os.Getenv("PATH"))

	aliveReports := make([]map[int][]string, 0)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/server/UniProxy/alive" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		var body map[int][]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode alive body: %v", err)
		}
		aliveReports = append(aliveReports, body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	api, err := panel.New(&conf.NodeConfig{APIHost: srv.URL, NodeID: 8, Key: "secret"})
	if err != nil {
		t.Fatalf("panel.New() error = %v", err)
	}
	info := &panel.NodeInfo{
		Id:   8,
		Type: "mieru",
		Tag:  "external-mieru:8",
		Common: &panel.CommonNode{Protocol: "mieru", ExternalProtocol: true, TrafficMode: "metrics", BaseConfig: &panel.BaseConfig{
			DeviceOnlineMinTraffic: 1,
		}},
	}
	controller := &Controller{
		apiClient:                api,
		info:                     info,
		tag:                      info.Tag,
		userList:                 []panel.UserInfo{{Id: 1001, Uuid: "uuid-1001"}},
		externalTrafficCollector: core.NewExternalTrafficCollector(info),
	}

	if err := os.WriteFile(metricsPath, []byte(`{"users":{"1001":{"DownloadBytes":5000,"UploadBytes":7000}}}`), 0644); err != nil {
		t.Fatalf("write first metrics: %v", err)
	}
	if err := controller.reportOnlineUsersTask(context.Background()); err != nil {
		t.Fatalf("first reportOnlineUsersTask() error = %v", err)
	}
	if len(aliveReports) != 0 {
		t.Fatalf("first online report should only establish baseline, got %#v", aliveReports)
	}

	if err := os.WriteFile(metricsPath, []byte(`{"users":{"1001":{"DownloadBytes":7000,"UploadBytes":8500}}}`), 0644); err != nil {
		t.Fatalf("write second metrics: %v", err)
	}
	if err := controller.reportOnlineUsersTask(context.Background()); err != nil {
		t.Fatalf("second reportOnlineUsersTask() error = %v", err)
	}
	if len(aliveReports) != 1 {
		t.Fatalf("alive report count = %d, want 1", len(aliveReports))
	}
	got := aliveReports[0][1001]
	if len(got) != 1 || got[0] != "external:mieru" {
		t.Fatalf("alive body for uid 1001 = %#v, want external:mieru", got)
	}
}

func TestReportSensitiveAccessTaskUsesJuicityObserverLog(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("V2NODE_EXTERNAL_CONFIG_DIR", tmp)
	reports := make([][]panel.SensitiveAccessEvent, 0)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/server/UniProxy/sensitive" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		var body []panel.SensitiveAccessEvent
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode sensitive body: %v", err)
		}
		reports = append(reports, body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	api, err := panel.New(&conf.NodeConfig{APIHost: srv.URL, NodeID: 7, Key: "secret"})
	if err != nil {
		t.Fatalf("panel.New() error = %v", err)
	}
	info := &panel.NodeInfo{Id: 7, Type: "juicity", Tag: "external-juicity:7", Common: &panel.CommonNode{Protocol: "juicity", ExternalProtocol: true, TrafficMode: "observer", SensitiveAudit: &panel.SensitiveAuditConfig{Enable: true}}}
	controller := &Controller{apiClient: api, info: info, tag: info.Tag, userList: []panel.UserInfo{{Id: 1001, Uuid: "uuid-1001"}}, externalTrafficCollector: core.NewExternalTrafficCollector(info)}
	line := `{"type":"access","uuid":"uuid-1001","domain":"blocked.example","rule":"suffix:example","client_ip":"203.0.113.9","count":1}` + "\n"
	if err := os.WriteFile(core.ExternalObservabilityLogPath(info), []byte(line), 0644); err != nil {
		t.Fatalf("write observer log: %v", err)
	}
	if err := controller.reportSensitiveAccessTask(context.Background()); err != nil {
		t.Fatalf("reportSensitiveAccessTask() error = %v", err)
	}
	if len(reports) != 1 || len(reports[0]) != 1 || reports[0][0].UserID != 1001 || reports[0][0].Domain != "blocked.example" {
		t.Fatalf("sensitive reports = %#v, want one event", reports)
	}
}
