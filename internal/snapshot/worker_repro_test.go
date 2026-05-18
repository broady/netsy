// SPDX-License-Identifier: Apache-2.0

package snapshot

import (
	"log/slog"
	"testing"
	"time"

	"github.com/netsy-dev/netsy/internal/config"
	"github.com/netsy-dev/netsy/internal/localdb"
	"github.com/netsy-dev/netsy/internal/proto"
)

type blockingSnapshotDB struct {
	localdb.Database
	started chan struct{}
	release chan struct{}
}

func (db *blockingSnapshotDB) FindAllRecordsForSnapshot(int64) ([]*proto.Record, error) {
	close(db.started)
	<-db.release
	return nil, nil
}

func TestWorkerStopDoesNotWaitForInFlightSnapshot(t *testing.T) {
	t.Parallel()

	db := &blockingSnapshotDB{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	worker := NewWorker(
		slog.Default(),
		&config.Config{
			NodeConfig: config.NodeConfig{
				DataDir: t.TempDir(),
			},
			ClusterConfig: config.ClusterConfig{
				Snapshot: config.SnapshotConfig{
					ThresholdRecords: 1,
				},
			},
		},
		db,
		nil,
		nil,
		nil,
	)
	worker.Start()
	worker.RequestSnapshot(1, time.Now(), 1)

	select {
	case <-db.started:
	case <-time.After(time.Second):
		t.Fatal("snapshot creation did not start")
	}

	stopped := make(chan struct{})
	go func() {
		worker.Stop()
		close(stopped)
	}()

	select {
	case <-stopped:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Worker.Stop() waited for the in-flight snapshot; failure condition was not reproduced")
	}

	close(db.release)
}
