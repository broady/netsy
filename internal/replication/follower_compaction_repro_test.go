// SPDX-License-Identifier: Apache-2.0

package replication

import (
	"log/slog"
	"testing"
	"time"

	"github.com/netsy-dev/netsy/internal/localdb"
	"github.com/netsy-dev/netsy/internal/nodestate"
)

type blockingCompactionDB struct {
	localdb.Database
	started chan struct{}
	release chan struct{}
}

func (db *blockingCompactionDB) PersistCompactionRevision(int64) error {
	return nil
}

func (db *blockingCompactionDB) ExecuteCompaction(int64) (int64, error) {
	close(db.started)
	<-db.release
	return 0, nil
}

func TestFollowerStopDoesNotWaitForInFlightCompaction(t *testing.T) {
	t.Parallel()

	db := &blockingCompactionDB{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	follower := &Follower{
		logger: slog.Default(),
		state:  nodestate.New(slog.Default()),
		db:     db,
		cancel: func() {},
	}

	if err := follower.handleCompact(10); err != nil {
		t.Fatalf("handleCompact() error = %v", err)
	}
	select {
	case <-db.started:
	case <-time.After(time.Second):
		t.Fatal("compaction execution did not start")
	}

	stopped := make(chan struct{})
	go func() {
		follower.Stop()
		close(stopped)
	}()

	select {
	case <-stopped:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Follower.Stop() waited for in-flight compaction; failure condition was not reproduced")
	}

	close(db.release)
}
