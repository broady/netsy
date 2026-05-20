// SPDX-License-Identifier: Apache-2.0

package clientapi

import (
	"context"
	"log/slog"
	"path/filepath"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/netsy-dev/netsy/internal/config"
	"github.com/netsy-dev/netsy/internal/localdb"
	"github.com/netsy-dev/netsy/internal/nodestate"
	"github.com/netsy-dev/netsy/internal/primary"
	"github.com/netsy-dev/netsy/internal/storage"
	pb "go.etcd.io/etcd/api/v3/etcdserverpb"
)

func TestApplyTxnReproducesSwallowedLeaderTxnError(t *testing.T) {
	t.Parallel()

	db := localdb.New(filepath.Join(t.TempDir(), "apply-txn.sqlite3"))
	if err := db.Connect(); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})

	failingDB := localdb.NewFailingDB(db)
	quorum := 0
	cfg := &config.Config{
		NodeConfig: config.NodeConfig{
			NodeID: "node-a",
		},
		ClusterConfig: config.ClusterConfig{
			Replication: config.ReplicationConfig{
				Quorum: &quorum,
			},
		},
	}

	state := nodestate.New(slog.Default())
	if err := state.SetPrimary(nodestate.PrimaryStarting); err != nil {
		t.Fatalf("SetPrimary(Starting) error = %v", err)
	}
	if err := state.SetPrimary(nodestate.PrimaryActive); err != nil {
		t.Fatalf("SetPrimary(Active) error = %v", err)
	}

	primarySrv, err := primary.NewServer(
		slog.Default(),
		cfg,
		failingDB,
		nil,
		storage.NewMemoryStore(),
		state,
		nil,
		nil,
		nil,
		0,
		0,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	failingDB.SetFailCommit(true)
	clientSrv := &ClientAPIServer{
		logger:     slog.Default(),
		db:         failingDB,
		state:      state,
		peerServer: primarySrv,
	}

	_, err = clientSrv.ApplyTxn(context.Background(), &pb.TxnRequest{
		Compare: []*pb.Compare{{
			Key:    []byte("key"),
			Target: pb.Compare_MOD,
			Result: pb.Compare_EQUAL,
			TargetUnion: &pb.Compare_ModRevision{
				ModRevision: 0,
			},
		}},
		Success: []*pb.RequestOp{{
			Request: &pb.RequestOp_RequestPut{
				RequestPut: &pb.PutRequest{
					Key:   []byte("key"),
					Value: []byte("value"),
				},
			},
		}},
	})
	if err == nil {
		t.Fatal("ApplyTxn() error = nil, want gRPC Internal error for LeaderTxn failure")
	}
	s, ok := status.FromError(err)
	if !ok || s.Code() != codes.Internal {
		t.Fatalf("ApplyTxn() error = %v, want gRPC status with code Internal", err)
	}
}
