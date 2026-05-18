// SPDX-License-Identifier: Apache-2.0

package primary

import (
	"context"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/netsy-dev/netsy/internal/config"
	"github.com/netsy-dev/netsy/internal/localdb"
	"github.com/netsy-dev/netsy/internal/nodestate"
	"github.com/netsy-dev/netsy/internal/storage"
	dto "github.com/prometheus/client_model/go"
)

func newMetricsReproServer(t *testing.T) *Server {
	t.Helper()

	db := localdb.New(filepath.Join(t.TempDir(), "metrics-repro.sqlite3"))
	if err := db.Connect(); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})

	quorum := 0
	state := nodestate.New(slog.Default())
	if err := state.SetPrimary(nodestate.PrimaryStarting); err != nil {
		t.Fatalf("SetPrimary(Starting) error = %v", err)
	}
	if err := state.SetPrimary(nodestate.PrimaryActive); err != nil {
		t.Fatalf("SetPrimary(Active) error = %v", err)
	}

	srv, err := NewServer(
		slog.Default(),
		&config.Config{
			NodeConfig: config.NodeConfig{
				NodeID: "node-a",
			},
			ClusterConfig: config.ClusterConfig{
				Replication: config.ReplicationConfig{
					Quorum: &quorum,
				},
			},
		},
		db,
		nil,
		storage.NewMemoryStore(),
		state,
		nil,
		nil,
		NewMetrics(),
		0,
		0,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}
	return srv
}

func TestCompareFailureIncrementsSuccessWriteMetric(t *testing.T) {
	t.Parallel()

	srv := newMetricsReproServer(t)
	ctx := context.Background()

	_, resp, err := srv.LeaderTxn(ctx, createTxnRequest("key", "old", 0))
	if err != nil {
		t.Fatalf("initial LeaderTxn() error = %v", err)
	}
	if !resp.GetSucceeded() {
		t.Fatal("initial LeaderTxn() Succeeded = false, want true")
	}

	_, resp, err = srv.LeaderTxn(ctx, createTxnRequest("key", "new", 99))
	if err != nil {
		t.Fatalf("compare-failed LeaderTxn() error = %v", err)
	}
	if resp.GetSucceeded() {
		t.Fatal("compare-failed LeaderTxn() Succeeded = true, want false")
	}

	metric := &dto.Metric{}
	if err := srv.metrics.WriteTransactions.WithLabelValues("sync", "success").Write(metric); err != nil {
		t.Fatalf("write metric: %v", err)
	}
	got := metric.GetCounter().GetValue()
	if got != 2 {
		t.Fatalf("sync/success write metric = %v, want 2 to reproduce compare failure counted as success", got)
	}
}
