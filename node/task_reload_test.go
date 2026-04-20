package node

import (
	"testing"
	"time"

	panel "github.com/wyx2685/v2node/api/v2board"
	vcore "github.com/wyx2685/v2node/core"
)

func TestStartTasksPassesReloadChannelToPeriodicTasks(t *testing.T) {
	t.Parallel()

	reloadCh := make(chan struct{}, 1)
	controller := &Controller{
		server: &vcore.V2Core{ReloadCh: reloadCh},
		info: &panel.NodeInfo{
			Common: &panel.CommonNode{},
		},
	}

	controller.startTasks(&panel.NodeInfo{
		PullInterval: 10 * time.Second,
		PushInterval: 10 * time.Second,
		Security:     panel.None,
	})
	defer controller.nodeInfoMonitorPeriodic.Close()
	defer controller.userReportPeriodic.Close()
	defer controller.onlineReportPeriodic.Close()

	if controller.nodeInfoMonitorPeriodic.ReloadCh != reloadCh {
		t.Fatal("nodeInfoMonitorPeriodic.ReloadCh was not wired")
	}
	if controller.userReportPeriodic.ReloadCh != reloadCh {
		t.Fatal("userReportPeriodic.ReloadCh was not wired")
	}
	if controller.onlineReportPeriodic.ReloadCh != reloadCh {
		t.Fatal("onlineReportPeriodic.ReloadCh was not wired")
	}
}
