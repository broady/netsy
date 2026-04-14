// Copyright 2026 Nadrama Pty Ltd
// SPDX-License-Identifier: Apache-2.0

package replication

import (
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/nadrama-com/netsy/internal/localdb"
	"github.com/nadrama-com/netsy/internal/nodestate"
	"github.com/nadrama-com/netsy/internal/proto"
)

type testWatchNotifier struct {
	committed []int64
}

// EnqueueWatchRevision records buffered revisions for test coverage.
func (n *testWatchNotifier) EnqueueWatchRevision(revision int64) {}

// AdvanceCommittedRevision records committed revisions seen by the notifier.
func (n *testWatchNotifier) AdvanceCommittedRevision(rev int64) {
	n.committed = append(n.committed, rev)
}

// ResetPending is a no-op for follower tests.
func (n *testWatchNotifier) ResetPending() {}

// newTestFollowerDB creates a temporary SQLite database suitable for follower
// tests that need real revision tracking.
func newTestFollowerDB(t *testing.T) localdb.Database {
	t.Helper()

	db := localdb.New(filepath.Join(t.TempDir(), "follower.db"))
	if err := db.Connect(); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})

	return db
}

// newTestFollower constructs a follower with a real SQLite database and no
// active peer connections.
func newTestFollower(t *testing.T) (*Follower, localdb.Database, *nodestate.State) {
	t.Helper()

	state := nodestate.New(slog.Default())
	if err := state.SetHealth(nodestate.HealthHealthy); err != nil {
		t.Fatalf("SetHealth(Healthy) error = %v", err)
	}

	db := newTestFollowerDB(t)
	follower := &Follower{
		logger: slog.Default(),
		state:  state,
		db:     db,
	}

	return follower, db, state
}

// TestHandleInitialDoesNotScheduleLagCheck verifies bootstrap Initial messages
// restore committed state without arming replica lag degradation while loading.
func TestHandleInitialDoesNotScheduleLagCheck(t *testing.T) {
	follower, _, state := newTestFollower(t)
	notifier := &testWatchNotifier{}
	follower.watchNotifier = notifier

	if err := state.SetHealth(nodestate.HealthDegraded); err != nil {
		t.Fatalf("SetHealth(Degraded) error = %v", err)
	}
	if err := state.SetHealth(nodestate.HealthLoading); err != nil {
		t.Fatalf("SetHealth(Loading) error = %v", err)
	}

	if err := follower.handleInitial(&proto.Initial{CommittedRevision: 5}); err != nil {
		t.Fatalf("handleInitial() error = %v", err)
	}

	time.Sleep(committedRevisionLagGracePeriod + 250*time.Millisecond)

	if state.Health() != nodestate.HealthLoading {
		t.Fatalf("Health() = %s, want %s", state.Health(), nodestate.HealthLoading)
	}
	if state.Committed() != 5 {
		t.Fatalf("Committed() = %d, want 5", state.Committed())
	}
	if len(notifier.committed) != 1 || notifier.committed[0] != 5 {
		t.Fatalf("watch notifier commits = %v, want [5]", notifier.committed)
	}
}
