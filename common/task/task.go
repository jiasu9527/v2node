package task

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

type Task struct {
	Name      string
	Interval  time.Duration
	Execute   func(context.Context) error
	Access    sync.RWMutex
	Running   bool
	ReloadCh  chan struct{}
	Stop      chan struct{}
	executing bool
	timedOut  bool

	RestartProcess func(reason string)
}

func (t *Task) Start(first bool) error {
	t.Access.Lock()
	if t.Running {
		t.Access.Unlock()
		return nil
	}
	t.Running = true
	t.Stop = make(chan struct{})
	t.Access.Unlock()
	go func() {
		defer t.safeStop()
		timer := time.NewTimer(t.Interval)
		defer timer.Stop()
		if first {
			if err := t.ExecuteWithTimeout(); err != nil {
				return
			}
		}

		for {
			timer.Reset(t.Interval)
			select {
			case <-timer.C:
				// continue
			case <-t.Stop:
				return
			}

			if err := t.ExecuteWithTimeout(); err != nil {
				log.Errorf("Task %s execution error: %v", t.Name, err)
				return
			}
		}
	}()

	return nil
}

func (t *Task) ExecuteWithTimeout() error {
	t.Access.Lock()
	if t.executing {
		timedOut := t.timedOut
		t.Access.Unlock()
		if timedOut {
			reason := fmt.Sprintf("Task %s remained executing after timeout", t.Name)
			log.Error(reason)
			t.restartProcess(reason)
			return nil
		}
		log.Warningf("Task %s previous run still executing, skipping current interval", t.Name)
		return nil
	}
	t.executing = true
	t.timedOut = false
	t.Access.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), min(5*t.Interval, 5*time.Minute))
	defer cancel()
	done := make(chan error, 1)

	go func() {
		defer func() {
			t.Access.Lock()
			t.executing = false
			t.timedOut = false
			t.Access.Unlock()
		}()
		done <- t.Execute(ctx)
	}()

	select {
	case <-ctx.Done():
		t.Access.Lock()
		t.timedOut = true
		t.Access.Unlock()
		log.Errorf("Task %s execution timed out, reloading", t.Name)
		if t.ReloadCh != nil {
			select {
			case t.ReloadCh <- struct{}{}:
			default:
			}
		} else {
			log.Panic("Reload failed")
		}
		return nil
	case err := <-done:
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil
		}
		return err
	}
}

func (t *Task) safeStop() {
	t.Access.Lock()
	if t.Running {
		t.Running = false
		close(t.Stop)
	}
	t.executing = false
	t.timedOut = false
	t.Access.Unlock()
}

func (t *Task) Close() {
	t.safeStop()
	log.Warningf("Task %s stopped", t.Name)
}

func (t *Task) restartProcess(reason string) {
	if t.RestartProcess != nil {
		t.RestartProcess(reason)
		return
	}
	log.Panic(reason)
}
