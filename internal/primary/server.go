// Copyright 2026 Nadrama Pty Ltd
// SPDX-License-Identifier: Apache-2.0

package primary

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/nadrama-com/netsy/internal/config"
	"github.com/nadrama-com/netsy/internal/localdb"
	"github.com/nadrama-com/netsy/internal/nodestate"
	"github.com/nadrama-com/netsy/internal/proto"
	"github.com/nadrama-com/netsy/internal/snapshot"
	"github.com/nadrama-com/netsy/internal/storage"
)

// Server implements the Primary domain layer and the proto.PrimaryServer
// gRPC interface. It handles transaction processing, the replication
// stream, and heartbeat collection from Replicas.
type Server struct {
	proto.UnimplementedPrimaryServer

	logger         *slog.Logger
	config         *config.Config
	db             localdb.Database
	storageClient  storage.ObjectStorage
	snapshotWorker *snapshot.Worker
	state          *nodestate.State
	replicas     *Replicas
	followMu     sync.RWMutex
	followStreams map[string]*followSession

	svcMu     sync.Mutex
	svcCancel context.CancelFunc

	heartbeatInterval time.Duration
	degradationCount  int

	// leaderTxnMutex serializes all transaction processing on the leader node.
	// This mutex should ONLY be used by the leader, not by follower nodes.
	leaderTxnMutex sync.Mutex

	// nextRevisionID holds the next revision ID to assign.
	// Managed atomically to ensure thread-safe access.
	nextRevisionID atomic.Int64
}

// NewServer constructs the Primary server and seeds its next revision
// counter from the database.
func NewServer(
	logger *slog.Logger,
	conf *config.Config,
	db localdb.Database,
	snapshotWorker *snapshot.Worker,
	storageClient storage.ObjectStorage,
	state *nodestate.State,
	heartbeatInterval time.Duration,
	degradationCount int,
) (*Server, error) {
	s := &Server{
		logger:            logger,
		config:            conf,
		db:                db,
		storageClient:     storageClient,
		snapshotWorker:    snapshotWorker,
		state:             state,
		replicas:          NewReplicas(),
		followStreams:     make(map[string]*followSession),
		heartbeatInterval: heartbeatInterval,
		degradationCount:  degradationCount,
	}

	if err := s.initializeRevisionCounter(); err != nil {
		return nil, err
	}

	return s, nil
}

// StartServices starts Primary background services (degradation loop).
// It is a no-op if services are already running.
func (s *Server) StartServices(parent context.Context) {
	s.svcMu.Lock()
	defer s.svcMu.Unlock()

	if s.svcCancel != nil {
		return
	}

	ctx, cancel := context.WithCancel(parent)
	s.svcCancel = cancel

	go s.RunDegradationLoop(ctx)
	s.logger.Info("primary services started")
}

// StopServices stops Primary background services and resets the follow
// hub and replica tracker. It is a no-op if services are not running.
func (s *Server) StopServices() {
	s.svcMu.Lock()
	defer s.svcMu.Unlock()

	if s.svcCancel == nil {
		return
	}

	s.svcCancel()
	s.svcCancel = nil
	s.resetFollowStreams()
	s.replicas.Reset()
	s.logger.Info("primary services stopped")
}

// Replicas returns the server's Replica tracker.
func (s *Server) Replicas() *Replicas {
	return s.replicas
}

// initializeRevisionCounter sets the next revision ID based on the highest
// revision currently in the database. This should only be called on leader
// startup.
func (s *Server) initializeRevisionCounter() error {
	latestRevision, err := s.db.LatestRevision()
	if err != nil {
		return err
	}
	s.nextRevisionID.Store(latestRevision + 1)
	return nil
}

// SendHeartbeat receives a standalone heartbeat from a Replica. The
// same processing code path is used for Receipt-embedded heartbeats
// via ReplicaMap.UpdateHeartbeat.
func (s *Server) SendHeartbeat(_ context.Context, req *proto.NodeState) (_ *emptypb.Empty, err error) {
	if err := s.requirePrimary(); err != nil {
		return nil, err
	}
	if req.GetNodeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "node_id is required")
	}

	health := nodestate.HealthFromProto(req.GetHealthState())
	primary := nodestate.PrimaryFromProto(req.GetPrimaryState())

	if !s.replicas.UpdateHeartbeat(req.GetNodeId(), health, primary, req.GetLatestRevision()) {
		return nil, status.Errorf(codes.NotFound, "replica %s is not connected", req.GetNodeId())
	}

	return &emptypb.Empty{}, nil
}

// requirePrimary returns a gRPC error if this node is not in a Primary
// state that accepts connections (Starting, Active, or Draining).
func (s *Server) requirePrimary() error {
	ps := s.state.Primary()
	switch ps {
	case nodestate.PrimaryStarting, nodestate.PrimaryActive, nodestate.PrimaryDraining:
		return nil
	default:
		return status.Errorf(codes.FailedPrecondition, "this node is not the primary (state: %s)", ps)
	}
}

// degradationCheckInterval is how often the degradation loop checks for
// Replicas that have missed heartbeats.
const degradationCheckInterval = 100 * time.Millisecond

// RunDegradationLoop periodically checks all Replicas and marks any as
// Degraded if they have missed the configured number of consecutive
// heartbeats. It runs until ctx is cancelled.
func (s *Server) RunDegradationLoop(ctx context.Context) {
	if s.heartbeatInterval == 0 {
		s.logger.Warn("replication heartbeat interval is 0, degradation checking disabled")
		return
	}

	ticker := time.NewTicker(degradationCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.checkDegradation()
		}
	}
}

// checkDegradation iterates over all Replicas and marks any as Degraded
// if their last heartbeat exceeds degradationCount * heartbeatInterval.
func (s *Server) checkDegradation() {
	deadline := time.Duration(s.degradationCount) * s.heartbeatInterval
	now := time.Now().UnixNano()
	entries := s.replicas.All()

	for _, entry := range entries {
		if entry.Health() == nodestate.HealthDegraded {
			continue
		}
		lastHB := entry.LastHeartbeat.Load()
		if time.Duration(now-lastHB) < deadline {
			continue
		}

		s.logger.Warn("marking replica degraded due to missed heartbeats",
			"node_id", entry.NodeID,
			"last_heartbeat_age", time.Duration(now-lastHB).String(),
			"deadline", deadline,
		)

		entry.SetHealth(nodestate.HealthDegraded)
	}
}

