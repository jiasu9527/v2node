package node

import (
	"context"
	"errors"
	"testing"

	panel "github.com/wyx2685/v2node/api/v2board"
)

func TestFilterObservationUsers(t *testing.T) {
	users := []panel.UserInfo{{Id: 1, Uuid: "one"}, {Id: 2, Uuid: "two"}}
	ordinary := &panel.NodeInfo{Common: &panel.CommonNode{Protocol: "anytls"}}
	got, err := filterObservationUsers(ordinary, users)
	if err != nil || len(got) != 2 {
		t.Fatalf("ordinary node changed users: %#v %v", got, err)
	}
	observed := &panel.NodeInfo{Common: &panel.CommonNode{Protocol: "anytls", ObservationAudit: &panel.ObservationAuditConfig{Enable: true, AllowedUserIDs: []int{2}}}}
	got, err = filterObservationUsers(observed, users)
	if err != nil || len(got) != 1 || got[0].Id != 2 {
		t.Fatalf("observation allowlist failed: %#v %v", got, err)
	}
	observed.Common.ObservationAudit.AllowedUserIDs = nil
	if got, err := filterObservationUsers(observed, users); err == nil || got != nil {
		t.Fatal("empty allowlist did not fail closed")
	}
	observed.Common.ObservationAudit.AllowedUserIDs = []int{3}
	if got, err := filterObservationUsers(observed, users); err != nil || got == nil || len(got) != 0 {
		t.Fatalf("all revoked users must produce a real empty list: %#v %v", got, err)
	}
}

func TestObservationRetryKeepsEventIDs(t *testing.T) {
	c := new(Controller)
	const total = 205
	events := make([]panel.ObservationEvent, total)
	for i := range events {
		events[i].EventID = string(rune('A' + i))
	}
	if dropped := c.appendPendingObservation(events); dropped != 0 {
		t.Fatalf("unexpected dropped count: %d", dropped)
	}
	want := errors.New("temporary unavailable")
	firstID := ""
	if err := c.flushPendingObservation(context.Background(), func(_ context.Context, batch []panel.ObservationEvent) error {
		firstID = batch[0].EventID
		return want
	}); !errors.Is(err, want) || len(c.pendingObservation) != total {
		t.Fatalf("failed batch was not retained: %v len=%d", err, len(c.pendingObservation))
	}
	var sizes []int
	if err := c.flushPendingObservation(context.Background(), func(_ context.Context, batch []panel.ObservationEvent) error {
		if len(sizes) == 0 && batch[0].EventID != firstID {
			t.Fatal("retry changed event ID")
		}
		sizes = append(sizes, len(batch))
		return nil
	}); err != nil || len(c.pendingObservation) != 0 {
		t.Fatalf("retry did not flush queue: %v remaining=%d", err, len(c.pendingObservation))
	}
	if len(sizes) != 3 || sizes[0] != 100 || sizes[1] != 100 || sizes[2] != 5 {
		t.Fatalf("wrong batch sizes: %#v", sizes)
	}
}

func TestObservationInvalidBatchDoesNotBlockLaterEvents(t *testing.T) {
	c := new(Controller)
	c.appendPendingObservation([]panel.ObservationEvent{{EventID: "good-1"}, {EventID: "bad"}, {EventID: "good-2"}})
	var accepted []string
	err := c.flushPendingObservation(context.Background(), func(_ context.Context, batch []panel.ObservationEvent) error {
		for _, event := range batch {
			if event.EventID == "bad" {
				return panel.ErrObservationInvalidBatch
			}
		}
		for _, event := range batch {
			accepted = append(accepted, event.EventID)
		}
		return nil
	})
	if err != nil || len(c.pendingObservation) != 0 || len(accepted) != 2 || accepted[0] != "good-1" || accepted[1] != "good-2" {
		t.Fatalf("invalid event blocked queue: err=%v pending=%d accepted=%#v", err, len(c.pendingObservation), accepted)
	}
}
