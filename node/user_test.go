package node

import (
	"reflect"
	"testing"

	panel "github.com/wyx2685/v2node/api/v2board"
)

func TestBuildOnlineUserReportDataFiltersByActiveUIDs(t *testing.T) {
	t.Parallel()

	onlineDevices := []panel.OnlineUser{
		{UID: 1, IP: "203.0.113.10"},
		{UID: 1, IP: "203.0.113.11"},
		{UID: 2, IP: "203.0.113.12"},
	}
	activeUIDs := map[int]struct{}{
		1: {},
	}

	got := buildOnlineUserReportData(onlineDevices, activeUIDs, true)
	want := map[int][]string{
		1: {"203.0.113.10", "203.0.113.11"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildOnlineUserReportData() = %#v, want %#v", got, want)
	}
}

func TestBuildOnlineUserReportDataReportsAllWithoutFilter(t *testing.T) {
	t.Parallel()

	onlineDevices := []panel.OnlineUser{
		{UID: 1, IP: "203.0.113.10"},
		{UID: 2, IP: "203.0.113.12"},
	}

	got := buildOnlineUserReportData(onlineDevices, nil, false)
	want := map[int][]string{
		1: {"203.0.113.10"},
		2: {"203.0.113.12"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildOnlineUserReportData() = %#v, want %#v", got, want)
	}
}
