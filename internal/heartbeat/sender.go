// Copyright 2026 Nadrama Pty Ltd
// SPDX-License-Identifier: Apache-2.0

package heartbeat

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nadrama-com/netsy/internal/nodestate"
	"github.com/nadrama-com/netsy/internal/peerclient"
	"github.com/nadrama-com/netsy/internal/proto"
)

// RevisionSource provides the latest revision for heartbeat messages.
type RevisionSource interface {
	LatestRevision() (int64, error)
}

// Sender sends heartbeats to the Elector and Primary on their respective
// cadences. It uses the following routing logic:
//
//   - To the Elector: always send on elector.heartbeat_interval.
//   - To the Primary: send on replication.heartbeat_interval only when
//     no Receipt has been sent within that window.
//   - When Elector == Primary: a single heartbeat satisfies both.
type Sender struct {
	logger              *slog.Logger
	nodeID              string
	state               *nodestate.State
	peers               *peerclient.Manager
	db                  RevisionSource
	startTime           int64
	electorInterval     time.Duration
	replicationInterval time.Duration

	// lastReceiptSent tracks when the last Receipt was sent to the
	// Primary, which should be updated atomically by the replication
	// layer. If zero, no Receipt has been sent yet.
	lastReceiptSent atomic.Int64

	// lastPrimaryNodeID tracks the Primary node ID from the previous
	// tick so we can clear lastReceiptSent on Primary changes.
	lastPrimaryMu     sync.Mutex
	lastPrimaryNodeID string
}

// NewSender creates a heartbeat Sender.
func NewSender(
	logger *slog.Logger,
	nodeID string,
	state *nodestate.State,
	peers *peerclient.Manager,
	db RevisionSource,
	startTime int64,
	electorInterval time.Duration,
	replicationInterval time.Duration,
) *Sender {
	return &Sender{
		logger:              logger,
		nodeID:              nodeID,
		state:               state,
		peers:               peers,
		db:                  db,
		startTime:           startTime,
		electorInterval:     electorInterval,
		replicationInterval: replicationInterval,
	}
}

// MarkReceiptSent records that a Receipt was just sent to the Primary,
// resetting the standalone heartbeat timer. Called by the replication
// stream (Follow) layer each time a Receipt is sent.
func (s *Sender) MarkReceiptSent() {
	s.lastReceiptSent.Store(time.Now().UnixNano())
}

// Run starts the heartbeat sender loops. It blocks until ctx is
// cancelled.
func (s *Sender) Run(ctx context.Context) {
	var wg sync.WaitGroup

	if s.electorInterval > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.runElectorLoop(ctx)
		}()
	}

	if s.replicationInterval > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.runPrimaryLoop(ctx)
		}()
	}

	wg.Wait()
}

func (s *Sender) runElectorLoop(ctx context.Context) {
	ticker := time.NewTicker(s.electorInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.sendToElector(ctx)
		}
	}
}

func (s *Sender) runPrimaryLoop(ctx context.Context) {
	ticker := time.NewTicker(s.replicationInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.sendToPrimaryIfNeeded(ctx)
		}
	}
}

func (s *Sender) sendToElector(ctx context.Context) {
	cs := s.state.ClusterState()
	client := s.peers.ElectorClient()

	if client == nil {
		return
	}

	ns := s.BuildNodeState()

	sendCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if _, err := client.SendHeartbeat(sendCtx, ns); err != nil {
		s.logger.Warn("failed to send heartbeat to elector", "error", err)
		return
	}

	// When Elector == Primary, this heartbeat satisfies both since the
	// server-side processing uses the same code path. Mark it so the
	// primary loop does not send a redundant heartbeat.
	if cs.Elector.NodeID != "" && cs.Elector.NodeID == cs.Primary.NodeID {
		s.lastReceiptSent.Store(time.Now().UnixNano())
	}
}

func (s *Sender) sendToPrimaryIfNeeded(ctx context.Context) {
	cs := s.state.ClusterState()

	// Clear lastReceiptSent when the Primary changes so a receipt to
	// the old Primary does not suppress the first heartbeat to the new one.
	s.lastPrimaryMu.Lock()
	if cs.Primary.NodeID != s.lastPrimaryNodeID {
		s.lastPrimaryNodeID = cs.Primary.NodeID
		s.lastReceiptSent.Store(0)
	}
	s.lastPrimaryMu.Unlock()

	// Skip when Elector == Primary (covered by elector heartbeat),
	// or when this node is the Primary.
	sameNode := cs.Elector.NodeID != "" && cs.Elector.NodeID == cs.Primary.NodeID
	isPrimary := cs.Primary.NodeID != "" && cs.Primary.NodeID == s.nodeID

	if sameNode || isPrimary {
		return
	}

	client := s.peers.PrimaryClient()
	if client == nil {
		return
	}

	// Only send if no Receipt (or elector heartbeat covering primary)
	// was sent within the replication interval.
	lastReceipt := s.lastReceiptSent.Load()
	if lastReceipt > 0 {
		elapsed := time.Duration(time.Now().UnixNano() - lastReceipt)
		if elapsed < s.replicationInterval {
			return
		}
	}

	ns := s.BuildNodeState()

	sendCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if _, err := client.SendHeartbeat(sendCtx, ns); err != nil {
		s.logger.Warn("failed to send heartbeat to primary", "error", err)
	}
}

func (s *Sender) BuildNodeState() *proto.NodeState {
	latestRevision, err := s.db.LatestRevision()
	if err != nil {
		s.logger.Warn("failed to get latest revision for heartbeat", "error", err)
	}

	return &proto.NodeState{
		NodeId:         s.nodeID,
		HealthState:    nodestate.HealthToProto(s.state.Health()),
		ElectorState:   nodestate.ElectorToProto(s.state.Elector()),
		PrimaryState:   nodestate.PrimaryToProto(s.state.Primary()),
		LatestRevision: latestRevision,
		StartTime:      s.startTime,
	}
}
