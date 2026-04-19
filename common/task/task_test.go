package task

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestExecuteWithTimeoutSkipsWhilePreviousRunStillExecuting(t *testing.T) {
	t.Parallel()

	var started atomic.Int32
	release := make(chan struct{})
	finished := make(chan struct{}, 1)

	task := &Task{
		Name:     "slow-task",
		Interval: 10 * time.Millisecond,
		Execute: func(context.Context) error {
			started.Add(1)
			<-release
			finished <- struct{}{}
			return nil
		},
	}

	begin := time.Now()
	if err := task.ExecuteWithTimeout(); err != nil {
		t.Fatalf("first ExecuteWithTimeout() error = %v", err)
	}
	if elapsed := time.Since(begin); elapsed < 25*time.Millisecond {
		t.Fatalf("first ExecuteWithTimeout() elapsed = %v, want timeout wait", elapsed)
	}
	if got := started.Load(); got != 1 {
		t.Fatalf("started after first timeout = %d, want 1", got)
	}

	begin = time.Now()
	if err := task.ExecuteWithTimeout(); err != nil {
		t.Fatalf("second ExecuteWithTimeout() error = %v", err)
	}
	if elapsed := time.Since(begin); elapsed > 10*time.Millisecond {
		t.Fatalf("second ExecuteWithTimeout() elapsed = %v, want immediate skip", elapsed)
	}
	if got := started.Load(); got != 1 {
		t.Fatalf("started after skip = %d, want 1", got)
	}

	close(release)

	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for first execution to finish")
	}

	begin = time.Now()
	if err := task.ExecuteWithTimeout(); err != nil {
		t.Fatalf("third ExecuteWithTimeout() error = %v", err)
	}
	if elapsed := time.Since(begin); elapsed > 10*time.Millisecond {
		t.Fatalf("third ExecuteWithTimeout() elapsed = %v, want prompt completion", elapsed)
	}
	if got := started.Load(); got != 2 {
		t.Fatalf("started after completed run = %d, want 2", got)
	}
}
