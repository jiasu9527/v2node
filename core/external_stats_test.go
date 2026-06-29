package core

import (
	"fmt"
	"os"
	"testing"

	mieruMetrics "github.com/enfein/mieru/v3/pkg/metrics"
	panel "github.com/wyx2685/v2node/api/v2board"
)

func TestMieruTrafficCollectorReportsDeltasFromInProcessMetrics(t *testing.T) {
	collector := NewExternalTrafficCollector(&panel.NodeInfo{Id: 8, Type: "mieru", Common: &panel.CommonNode{Protocol: "mieru"}})
	users := []panel.UserInfo{{Id: 910001, Uuid: "uuid-910001"}, {Id: 910002, Uuid: "uuid-910002"}}

	up1 := mieruMetrics.RegisterMetric(fmt.Sprintf(mieruMetrics.UserMetricGroupFormat, "910001"), mieruMetrics.UserMetricUploadBytes, mieruMetrics.COUNTER_TIME_SERIES).(*mieruMetrics.Counter)
	down1 := mieruMetrics.RegisterMetric(fmt.Sprintf(mieruMetrics.UserMetricGroupFormat, "910001"), mieruMetrics.UserMetricDownloadBytes, mieruMetrics.COUNTER_TIME_SERIES).(*mieruMetrics.Counter)
	up2 := mieruMetrics.RegisterMetric(fmt.Sprintf(mieruMetrics.UserMetricGroupFormat, "910002"), mieruMetrics.UserMetricUploadBytes, mieruMetrics.COUNTER_TIME_SERIES).(*mieruMetrics.Counter)
	down2 := mieruMetrics.RegisterMetric(fmt.Sprintf(mieruMetrics.UserMetricGroupFormat, "910002"), mieruMetrics.UserMetricDownloadBytes, mieruMetrics.COUNTER_TIME_SERIES).(*mieruMetrics.Counter)

	up1.Add(7000)
	down1.Add(5000)
	up2.Add(200)
	down2.Add(100)
	got, err := collector.CollectTraffic(users, 0)
	if err != nil {
		t.Fatalf("first CollectTraffic() error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("first CollectTraffic() = %#v, want baseline only", got)
	}

	up1.Add(2000)
	down1.Add(4000)
	up2.Add(50)
	down2.Add(50)
	got, err = collector.CollectTraffic(users, 0)
	if err != nil {
		t.Fatalf("second CollectTraffic() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("second CollectTraffic() len = %d, want 2: %#v", len(got), got)
	}
	byUID := map[int]panel.UserTraffic{}
	for _, traffic := range got {
		byUID[traffic.UID] = traffic
	}
	if byUID[910001].Upload != 2000 || byUID[910001].Download != 4000 {
		t.Fatalf("uid 910001 delta = %#v, want up=2000 down=4000", byUID[910001])
	}
	if byUID[910002].Upload != 50 || byUID[910002].Download != 50 {
		t.Fatalf("uid 910002 delta = %#v, want up=50 down=50", byUID[910002])
	}
}

func TestMieruTrafficCollectorHonorsMinTrafficKilobytes(t *testing.T) {
	collector := &ExternalTrafficCollector{protocol: "mieru", prev: map[int]externalTrafficSnapshot{1: {Upload: 1000, Download: 1000}}}
	snapshots := map[int]externalTrafficSnapshot{1: {Upload: 1300, Download: 1400}}
	got := collector.deltaTraffic(snapshots, 1)
	if len(got) != 0 {
		t.Fatalf("deltaTraffic below min = %#v, want empty", got)
	}
	got = collector.deltaTraffic(snapshots, 0)
	if len(got) != 1 || got[0].Upload != 300 || got[0].Download != 400 {
		t.Fatalf("deltaTraffic no min = %#v, want one delta", got)
	}
}

func TestJuicityTrafficCollectorWithoutObserverLogReturnsEmpty(t *testing.T) {
	t.Setenv("V2NODE_EXTERNAL_CONFIG_DIR", t.TempDir())
	collector := NewExternalTrafficCollector(&panel.NodeInfo{Id: 7, Type: "juicity", Common: &panel.CommonNode{Protocol: "juicity"}})
	got, err := collector.CollectTraffic([]panel.UserInfo{{Id: 1, Uuid: "uuid"}}, 0)
	if err != nil {
		t.Fatalf("CollectTraffic() error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("CollectTraffic() = %#v, want empty without observer log", got)
	}
}

func TestMieruTrafficCollectorCollectOnlineUsersFromMetricDeltas(t *testing.T) {
	collector := &ExternalTrafficCollector{protocol: "mieru", prev: map[int]externalTrafficSnapshot{1001: {Upload: 1000, Download: 1000}, 1002: {Upload: 2000, Download: 2000}}}
	snapshots := map[int]externalTrafficSnapshot{1001: {Upload: 1000, Download: 1000}, 1002: {Upload: 2800, Download: 2500}}
	got := collector.deltaOnlineUsers(snapshots, 1)
	if len(got) != 1 || got[0].UID != 1002 || got[0].IP != "external:mieru" {
		t.Fatalf("deltaOnlineUsers() = %#v, want uid 910002 external:mieru", got)
	}
}

func TestMieruTrafficCollectorOnlineKeepsTrafficBaselineForLaterReport(t *testing.T) {
	collector := &ExternalTrafficCollector{protocol: "mieru", prev: map[int]externalTrafficSnapshot{1001: {Upload: 1000, Download: 1000}}}
	snapshots := map[int]externalTrafficSnapshot{1001: {Upload: 1300, Download: 1400}}
	if got := collector.deltaOnlineUsers(snapshots, 0); len(got) != 1 {
		t.Fatalf("deltaOnlineUsers() len = %d, want 1", len(got))
	}
	traffic := collector.deltaTraffic(snapshots, 0)
	if len(traffic) != 1 || traffic[0].Upload != 300 || traffic[0].Download != 400 {
		t.Fatalf("deltaTraffic after online = %#v, want unchanged traffic delta", traffic)
	}
}

func TestJuicityObserverLogReportsTrafficAndOnline(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("V2NODE_EXTERNAL_CONFIG_DIR", tmp)
	node := &panel.NodeInfo{Id: 7, Type: "juicity", Common: &panel.CommonNode{Protocol: "juicity"}}
	collector := NewExternalTrafficCollector(node)
	users := []panel.UserInfo{{Id: 1001, Uuid: "uuid-1001"}}
	logPath := ExternalObservabilityLogPath(node)
	if err := os.WriteFile(logPath, []byte(`{"type":"traffic","uuid":"uuid-1001","upload":1234,"download":5678,"client_ip":"203.0.113.9"}`+"\n"), 0644); err != nil {
		t.Fatalf("write observer log: %v", err)
	}

	traffic, err := collector.CollectTraffic(users, 0)
	if err != nil {
		t.Fatalf("CollectTraffic() error = %v", err)
	}
	if len(traffic) != 1 || traffic[0].UID != 1001 || traffic[0].Upload != 1234 || traffic[0].Download != 5678 {
		t.Fatalf("CollectTraffic() = %#v, want uid 910001 up/down", traffic)
	}
	online, err := collector.CollectOnlineUsers(users, 0)
	if err != nil {
		t.Fatalf("CollectOnlineUsers() error = %v", err)
	}
	if len(online) != 1 || online[0].UID != 1001 || online[0].IP != "203.0.113.9" {
		t.Fatalf("CollectOnlineUsers() = %#v, want uid 910001 ip", online)
	}

	traffic, err = collector.CollectTraffic(users, 0)
	if err != nil || len(traffic) != 0 {
		t.Fatalf("second CollectTraffic() = %#v, err=%v; want no duplicate", traffic, err)
	}
}

func TestJuicityObserverLogReportsSensitiveEvents(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("V2NODE_EXTERNAL_CONFIG_DIR", tmp)
	node := &panel.NodeInfo{Id: 7, Type: "juicity", Common: &panel.CommonNode{Protocol: "juicity"}}
	collector := NewExternalTrafficCollector(node)
	users := []panel.UserInfo{{Id: 1001, Uuid: "uuid-1001"}}
	line := `{"type":"access","uuid":"uuid-1001","domain":"blocked.example","rule":"suffix:example","client_ip":"203.0.113.9","count":2,"first_at":11,"last_at":22}` + "\n"
	if err := os.WriteFile(ExternalObservabilityLogPath(node), []byte(line), 0644); err != nil {
		t.Fatalf("write observer log: %v", err)
	}

	events, err := collector.CollectSensitiveAccess(users)
	if err != nil {
		t.Fatalf("CollectSensitiveAccess() error = %v", err)
	}
	if len(events) != 1 || events[0].UserID != 1001 || events[0].Domain != "blocked.example" || events[0].ClientIP != "203.0.113.9" || events[0].Count != 2 {
		t.Fatalf("CollectSensitiveAccess() = %#v, want one event", events)
	}
}
