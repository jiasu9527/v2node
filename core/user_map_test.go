package core

import "testing"

func TestUserMapSnapshotReturnsIndependentCopy(t *testing.T) {
	t.Parallel()

	users := &UserMap{
		uidMap: map[string]int{
			"user-a": 1,
			"user-b": 2,
		},
	}

	snapshot := users.Snapshot()
	if len(snapshot) != 2 {
		t.Fatalf("snapshot size = %d, want 2", len(snapshot))
	}

	snapshot["user-a"] = 100
	delete(snapshot, "user-b")

	if got := users.uidMap["user-a"]; got != 1 {
		t.Fatalf("original user-a = %d, want 1", got)
	}
	if got := users.uidMap["user-b"]; got != 2 {
		t.Fatalf("original user-b = %d, want 2", got)
	}
}
