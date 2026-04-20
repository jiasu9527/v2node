package task

import (
	"context"
	"testing"
	"time"
)

func TestExecuteWithTimeoutSkipsWhilePreviousRunStillExecuting(t *testing.T) {
	t.Parallel()

	restartCh := make(chan string, 1)

	task := &Task{
		Name:           "slow-task",
		Interval:       10 * time.Millisecond,
		ReloadCh:       make(chan struct{}, 1),
		executing:      true,
		RestartProcess: func(reason string) { restartCh <- reason },
		Execute: func(context.Context) error {
			t.Fatal("Execute should not be invoked when task is already executing")
			return nil
		},
	}

	begin := time.Now()
	if err := task.ExecuteWithTimeout(); err != nil {
		t.Fatalf("ExecuteWithTimeout() error = %v", err)
	}
	if elapsed := time.Since(begin); elapsed > 10*time.Millisecond {
		t.Fatalf("ExecuteWithTimeout() elapsed = %v, want immediate skip", elapsed)
	}
	select {
	case reason := <-restartCh:
		t.Fatalf("unexpected restart request: %q", reason)
	case <-time.After(20 * time.Millisecond):
	}
}

func TestExecuteWithTimeoutSignalsReloadOnTimeout(t *testing.T) {
	t.Parallel()

	reloadCh := make(chan struct{}, 1)
	release := make(chan struct{})
	task := &Task{
		Name:     "reload-on-timeout",
		Interval: 10 * time.Millisecond,
		ReloadCh: reloadCh,
		Execute: func(context.Context) error {
			<-release
			return nil
		},
	}

	if err := task.ExecuteWithTimeout(); err != nil {
		t.Fatalf("ExecuteWithTimeout() error = %v", err)
	}

	select {
	case <-reloadCh:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected reload signal after timeout")
	}

	close(release)
	deadline := time.After(time.Second)
	for {
		task.Access.RLock()
		executing := task.executing
		task.Access.RUnlock()
		if !executing {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for timed-out execution to exit")
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func TestExecuteWithTimeoutRestartsWhenTimedOutRunKeepsExecuting(t *testing.T) {
	t.Parallel()

	reloadCh := make(chan struct{}, 1)
	restartCh := make(chan string, 1)
	release := make(chan struct{})
	task := &Task{
		Name:     "restart-after-stuck-timeout",
		Interval: 10 * time.Millisecond,
		ReloadCh: reloadCh,
		Execute: func(context.Context) error {
			<-release
			return nil
		},
		RestartProcess: func(reason string) {
			restartCh <- reason
		},
	}

	if err := task.ExecuteWithTimeout(); err != nil {
		t.Fatalf("first ExecuteWithTimeout() error = %v", err)
	}

	select {
	case <-reloadCh:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected reload signal after timeout")
	}

	if err := task.ExecuteWithTimeout(); err != nil {
		t.Fatalf("second ExecuteWithTimeout() error = %v", err)
	}

	select {
	case reason := <-restartCh:
		if reason == "" {
			t.Fatal("expected non-empty restart reason")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected process restart after stuck timeout")
	}

	close(release)
}
