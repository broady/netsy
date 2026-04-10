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
	db                  RevisionSource
	startTime           int64
	electorInterval     time.Duration
	replicationInterval time.Duration

	// lastReceiptSent tracks when the last Receipt was sent to the
	// Primary, which should be updated atomically by the replication
	// layer. If zero, no Receipt has been sent yet.
	lastReceiptSent atomic.Int64

	mu            sync.RWMutex
	electorClient proto.ElectorClient
	electorNodeID string
	primaryClient proto.PrimaryClient
	primaryNodeID string
	sameNode      bool // true when Elector and Primary are the same Node
	isPrimary     bool // true when this Node is the Primary (skip self-heartbeats)

	resetElector chan struct{}
	resetPrimary chan struct{}
}

// NewSender creates a heartbeat Sender. Elector and Primary targets are
// set later via SetElector / SetPrimary as connections are established
// or cluster state changes.
func NewSender(
	logger *slog.Logger,
	nodeID string,
	state *nodestate.State,
	db RevisionSource,
	startTime int64,
	electorInterval time.Duration,
	replicationInterval time.Duration,
) *Sender {
	return &Sender{
		logger:              logger,
		nodeID:              nodeID,
		state:               state,
		db:                  db,
		startTime:           startTime,
		electorInterval:     electorInterval,
		replicationInterval: replicationInterval,
		resetElector:        make(chan struct{}, 1),
		resetPrimary:        make(chan struct{}, 1),
	}
}

// SetElector updates the Elector target connection and node ID. Called
// when the Elector changes via a cluster state update. Resets the
// elector heartbeat timer.
func (s *Sender) SetElector(electorNodeID string, client proto.ElectorClient) {
	s.mu.Lock()
	s.electorClient = client
	s.electorNodeID = electorNodeID
	s.sameNode = electorNodeID != "" && electorNodeID == s.primaryNodeID
	s.mu.Unlock()

	select {
	case s.resetElector <- struct{}{}:
	default:
	}
}

// SetPrimary updates the Primary target connection and node ID. Called
// when the Primary changes via a cluster state update. Resets the
// primary heartbeat timer.
func (s *Sender) SetPrimary(primaryNodeID string, client proto.PrimaryClient) {
	s.mu.Lock()
	s.primaryClient = client
	s.primaryNodeID = primaryNodeID
	s.sameNode = s.electorNodeID != "" && s.electorNodeID == primaryNodeID
	s.isPrimary = primaryNodeID != "" && primaryNodeID == s.nodeID
	s.mu.Unlock()

	select {
	case s.resetPrimary <- struct{}{}:
	default:
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
		case <-s.resetElector:
			ticker.Reset(s.electorInterval)
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
		case <-s.resetPrimary:
			ticker.Reset(s.replicationInterval)
		case <-ticker.C:
			s.sendToPrimaryIfNeeded(ctx)
		}
	}
}

func (s *Sender) sendToElector(ctx context.Context) {
	s.mu.RLock()
	client := s.electorClient
	sameNode := s.sameNode
	s.mu.RUnlock()

	if client == nil {
		return
	}

	ns := s.buildNodeState()

	sendCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if _, err := client.SendHeartbeat(sendCtx, ns); err != nil {
		s.logger.Warn("failed to send heartbeat to elector", "error", err)
		return
	}

	// When Elector == Primary, this heartbeat satisfies both since the
	// server-side processing uses the same code path. Mark it so the
	// primary loop does not send a redundant heartbeat.
	if sameNode {
		s.lastReceiptSent.Store(time.Now().UnixNano())
	}
}

func (s *Sender) sendToPrimaryIfNeeded(ctx context.Context) {
	s.mu.RLock()
	client := s.primaryClient
	sameNode := s.sameNode
	isPrimary := s.isPrimary
	s.mu.RUnlock()

	if client == nil || sameNode || isPrimary {
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

	ns := s.buildNodeState()

	sendCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if _, err := client.SendHeartbeat(sendCtx, ns); err != nil {
		s.logger.Warn("failed to send heartbeat to primary", "error", err)
	}
}

func (s *Sender) buildNodeState() *proto.NodeState {
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
