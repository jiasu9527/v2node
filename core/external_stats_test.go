package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	panel "github.com/wyx2685/v2node/api/v2board"
)

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func TestMieruTrafficCollectorReportsDeltasFromMetricsJSON(t *testing.T) {
	tmp := t.TempDir()
	calls := filepath.Join(tmp, "calls")
	metrics := filepath.Join(tmp, "metrics.json")
	bin := filepath.Join(tmp, "mita")
	script := "#!/usr/bin/env sh\n" +
		"echo \"$@\" >> " + shellQuote(calls) + "\n" +
		"if [ \"$1 $2\" = \"get metrics\" ]; then cat " + shellQuote(metrics) + "; exit 0; fi\n" +
		"exit 0\n"
	if err := os.WriteFile(bin, []byte(script), 0755); err != nil {
		t.Fatalf("write fake mita: %v", err)
	}
	t.Setenv("PATH", tmp+string(os.PathListSeparator)+os.Getenv("PATH"))

	collector := NewExternalTrafficCollector(&panel.NodeInfo{Id: 8, Type: "mieru", Common: &panel.CommonNode{Protocol: "mieru"}})
	users := []panel.UserInfo{{Id: 1001, Uuid: "uuid-1001"}, {Id: 1002, Uuid: "uuid-1002"}}

	first := `{
        "users": {
            "1001": {"DownloadBytes": 5000, "UploadBytes": 7000},
            "1002": {"DownloadBytes": 100, "UploadBytes": 200}
        }
    }`
	if err := os.WriteFile(metrics, []byte(first), 0644); err != nil {
		t.Fatalf("write metrics: %v", err)
	}
	got, err := collector.CollectTraffic(users, 0)
	if err != nil {
		t.Fatalf("first CollectTraffic() error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("first CollectTraffic() = %#v, want baseline only", got)
	}

	second := `{
        "users": {
            "1001": {"DownloadBytes": 9000, "UploadBytes": 9000},
            "1002": {"DownloadBytes": 150, "UploadBytes": 250}
        }
    }`
	if err := os.WriteFile(metrics, []byte(second), 0644); err != nil {
		t.Fatalf("write metrics: %v", err)
	}
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
	if byUID[1001].Upload != 2000 || byUID[1001].Download != 4000 {
		t.Fatalf("uid 1001 delta = %#v, want up=2000 down=4000", byUID[1001])
	}
	if byUID[1002].Upload != 50 || byUID[1002].Download != 50 {
		t.Fatalf("uid 1002 delta = %#v, want up=50 down=50", byUID[1002])
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

func TestJuicityTrafficCollectorReturnsUnsupported(t *testing.T) {
	collector := NewExternalTrafficCollector(&panel.NodeInfo{Id: 7, Type: "juicity", Common: &panel.CommonNode{Protocol: "juicity"}})
	_, err := collector.CollectTraffic([]panel.UserInfo{{Id: 1, Uuid: "uuid"}}, 0)
	if err == nil {
		t.Fatal("CollectTraffic() error = nil, want unsupported error")
	}
}
