// Copyright 2026 Nadrama Pty Ltd
// SPDX-License-Identifier: Apache-2.0

package clientapi

import (
	"log/slog"
	"sort"
	"sync"

	"github.com/nadrama-com/netsy/internal/config"
	"github.com/nadrama-com/netsy/internal/localdb"
	"github.com/nadrama-com/netsy/internal/nodestate"
	"github.com/nadrama-com/netsy/internal/primary"
	internalproto "github.com/nadrama-com/netsy/internal/proto"

	pb "go.etcd.io/etcd/api/v3/etcdserverpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

// ClientAPIServer implements a gRPC server compatible with the Kubernetes etcd API subset
// @see https://github.com/etcd-io/etcd/blob/main/api/etcdserverpb/rpc.proto#L37
// @see https://github.com/etcd-io/etcd/blob/main/api/etcdserverpb/rpc.pb.go
// etcd has the following gRPC "services":
// * KV
// * Watch
// * Lease
// * Cluster
// * Maintenance
// * Auth
// we include the 'Unimplemented' services by default and override them where required
type ClientAPIServer struct {
	logger     *slog.Logger
	config     *config.Config
	db         localdb.Database
	grpcServer *grpc.Server
	state *nodestate.State
	// note: in future we will replace this with a peer server gRPC client
	peerServer *primary.Server

	// pendingMu guards the set of revisions waiting for committed_revision
	// to advance before being delivered to watchers. Only revision numbers
	// are stored; the full record is read from SQLite at delivery time.
	pendingMu sync.Mutex
	pending   map[int64]struct{}

	pb.UnimplementedKVServer
	pb.UnimplementedWatchServer
	pb.UnimplementedLeaseServer
	pb.UnimplementedClusterServer
	pb.UnimplementedMaintenanceServer
	pb.UnimplementedAuthServer
}

// NewServer registers the etcd-compatible Client API services on the provided gRPC server.
func NewServer(logger *slog.Logger, conf *config.Config, db localdb.Database, grpcServer *grpc.Server, peerServer *primary.Server, state *nodestate.State) *ClientAPIServer {
	clientServer := &ClientAPIServer{
		logger:     logger,
		config:     conf,
		grpcServer: grpcServer,
		db:         db,
		state:      state,
		peerServer: peerServer,
		pending:    make(map[int64]struct{}),
	}

	pb.RegisterKVServer(grpcServer, clientServer)
	pb.RegisterWatchServer(grpcServer, clientServer)
	pb.RegisterLeaseServer(grpcServer, clientServer)
	pb.RegisterClusterServer(grpcServer, clientServer)
	pb.RegisterMaintenanceServer(grpcServer, clientServer)
	pb.RegisterAuthServer(grpcServer, clientServer)
	hsrv := health.NewServer()
	hsrv.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(grpcServer, hsrv)
	reflection.Register(grpcServer)

	return clientServer
}

func (clientServer *ClientAPIServer) Close() {
	clientServer.grpcServer.GracefulStop()
	clientServer.db.Close()
}

// EnqueueWatchRevision buffers a revision for watch delivery once
// committed_revision advances past it. Used by Replicas that receive
// records before the corresponding commit message.
func (cs *ClientAPIServer) EnqueueWatchRevision(revision int64) {
	if revision <= 0 {
		return
	}

	cs.pendingMu.Lock()
	cs.pending[revision] = struct{}{}
	cs.pendingMu.Unlock()
}

// ResetPending discards all buffered revisions. Called when the
// replication stream reconnects, since any pending entries from the
// previous stream are stale.
func (cs *ClientAPIServer) ResetPending() {
	cs.pendingMu.Lock()
	cs.pending = make(map[int64]struct{})
	cs.pendingMu.Unlock()
}

// AdvanceCommittedRevision delivers all pending watch events with
// revisions up to and including rev, in ascending revision order.
func (cs *ClientAPIServer) AdvanceCommittedRevision(rev int64) {
	cs.pendingMu.Lock()

	var ready []int64
	for r := range cs.pending {
		if r <= rev {
			ready = append(ready, r)
		}
	}

	if len(ready) == 0 {
		cs.pendingMu.Unlock()
		return
	}

	sort.Slice(ready, func(i, j int) bool { return ready[i] < ready[j] })

	for _, r := range ready {
		delete(cs.pending, r)
	}
	cs.pendingMu.Unlock()

	for _, r := range ready {
		cs.distributeFromDB(r)
	}
}

// distributeFromDB reads a record from SQLite by revision and delivers
// it to matching watchers, including the previous record for watches
// that request prev_kv.
func (cs *ClientAPIServer) distributeFromDB(revision int64) {
	record, err := cs.db.FindRecordByRev(revision)
	if err != nil {
		cs.logger.Warn("failed to read record for watch delivery", "revision", revision, "error", err)
		return
	}

	var prevRecord *internalproto.Record
	if !record.Created && record.PrevRevision > 0 {
		prevRecord, err = cs.db.FindRecordByRev(record.PrevRevision)
		if err != nil {
			cs.logger.Debug("failed to read prev record for watch delivery", "revision", revision, "prev_revision", record.PrevRevision, "error", err)
		}
	}

	cs.Distribute(record, prevRecord)
}
