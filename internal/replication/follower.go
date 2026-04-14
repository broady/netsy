// Copyright 2026 Nadrama Pty Ltd
// SPDX-License-Identifier: Apache-2.0

package replication

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/nadrama-com/netsy/internal/heartbeat"
	"github.com/nadrama-com/netsy/internal/localdb"
	"github.com/nadrama-com/netsy/internal/nodestate"
	"github.com/nadrama-com/netsy/internal/peerclient"
	"github.com/nadrama-com/netsy/internal/proto"
)

// reconnectDelay is the wait time before retrying a Follow stream
// connection after a failure.
const reconnectDelay = time.Second

// WatchNotifier is called when the Follower receives records or commit
// messages that should be forwarded to the watch subsystem.
type WatchNotifier interface {
	EnqueueWatchRevision(revision int64)
	AdvanceCommittedRevision(rev int64)
	ResetPending()
}

// Follower connects to the Primary's Follow RPC as a Replica, receives
// PrimaryMessages, persists records locally, and sends Receipts.
type Follower struct {
	logger          *slog.Logger
	nodeID          string
	state           *nodestate.State
	peers           *peerclient.Manager
	db              localdb.Database
	heartbeatSender *heartbeat.Sender
	watchNotifier   WatchNotifier

	mu     sync.Mutex
	cancel context.CancelFunc
}

// NewFollower creates a new replication Follower.
func NewFollower(
	logger *slog.Logger,
	nodeID string,
	state *nodestate.State,
	peers *peerclient.Manager,
	db localdb.Database,
	heartbeatSender *heartbeat.Sender,
	watchNotifier WatchNotifier,
) *Follower {
	return &Follower{
		logger:          logger,
		nodeID:          nodeID,
		state:           state,
		peers:           peers,
		db:              db,
		heartbeatSender: heartbeatSender,
		watchNotifier:   watchNotifier,
	}
}

// Start begins the Follow stream loop in a background goroutine. It
// is a no-op if the follower is already running. The provided context
// is used as the parent; call Stop to cancel the follower.
func (f *Follower) Start(parent context.Context) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.cancel != nil {
		return
	}

	ctx, cancel := context.WithCancel(parent)
	f.cancel = cancel

	go f.run(ctx)
	f.logger.Info("follower started")
}

// Stop cancels the Follow stream loop. It is a no-op if the follower
// is not running.
func (f *Follower) Stop() {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.cancel == nil {
		return
	}

	f.cancel()
	f.cancel = nil
	f.logger.Info("follower stopped")
}

// run is the internal reconnect loop.
func (f *Follower) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if err := f.connectAndFollow(ctx); err != nil {
			f.logger.Warn("follow stream ended", "error", err)
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(reconnectDelay):
		}
	}
}

// connectAndFollow attempts a single Follow stream session. It returns
// when the stream ends or an error occurs.
func (f *Follower) connectAndFollow(ctx context.Context) error {
	cs := f.state.ClusterState()

	// Skip if no Primary, or if this node is the Primary.
	if cs.Primary.NodeID == "" || cs.Primary.NodeID == f.nodeID {
		return nil
	}

	// Validate the Primary's state before streaming. Replicas must
	// reject data from a node whose Primary State is Replica.
	remoteState, err := f.peers.GetNodeState(ctx, cs.Primary.PeerAdvertiseAddr)
	if err != nil {
		return err
	}
	remotePrimary := nodestate.PrimaryFromProto(remoteState.GetPrimaryState())
	if remotePrimary == nodestate.PrimaryReplica {
		f.logger.Warn("remote node is not primary, skipping follow",
			"remote_node_id", cs.Primary.NodeID,
			"remote_primary_state", remotePrimary,
		)
		return nil
	}

	client := f.peers.PrimaryClient()
	if client == nil {
		return nil
	}

	stream, err := client.Follow(ctx)
	if err != nil {
		return err
	}

	f.logger.Info("follow stream established", "primary", cs.Primary.NodeID)

	// Discard any pending watch events from a previous stream since
	// they are stale — the new Initial message will set the
	// authoritative committed revision.
	if f.watchNotifier != nil {
		f.watchNotifier.ResetPending()
	}

	return f.processStream(stream)
}

// processStream handles all messages on an active Follow stream.
func (f *Follower) processStream(stream proto.Primary_FollowClient) error {
	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		switch payload := msg.Payload.(type) {
		case *proto.PrimaryMessage_Initial:
			f.handleInitial(payload.Initial)

		case *proto.PrimaryMessage_Record:
			if err := f.handleRecord(stream, payload.Record); err != nil {
				return err
			}

		case *proto.PrimaryMessage_Commit:
			f.handleCommit(payload.Commit)

		case *proto.PrimaryMessage_Compact:
			f.handleCompact(payload.Compact)
		}
	}
}

// handleInitial processes the Initial message sent by the Primary at
// the start of each Follow stream, seeding committed and compaction
// revisions on this node.
func (f *Follower) handleInitial(initial *proto.Initial) {
	f.state.SetCommitted(initial.GetCommittedRevision())
	f.state.SetCompaction(initial.GetCompactionRevision())

	f.logger.Info("received initial",
		"committed_revision", initial.GetCommittedRevision(),
		"compaction_revision", initial.GetCompactionRevision(),
	)

	if f.watchNotifier != nil {
		f.watchNotifier.AdvanceCommittedRevision(initial.GetCommittedRevision())
	}
}

// handleRecord persists a replicated record to the local database,
// enqueues the revision for watch delivery, and sends a receipt with
// an embedded heartbeat back to the Primary.
func (f *Follower) handleRecord(stream proto.Primary_FollowClient, record *proto.Record) error {
	// ReplicateRecord sets replicated_at on the record.
	if _, err := f.db.ReplicateRecord(record); err != nil {
		f.logger.Error("failed to replicate record", "revision", record.GetRevision(), "error", err)
		return err
	}

	// Buffer the revision for watch delivery — it will be read from
	// the database and delivered when the commit message arrives.
	if f.watchNotifier != nil {
		f.watchNotifier.EnqueueWatchRevision(record.GetRevision())
	}

	// Send receipt with embedded heartbeat back to the Primary.
	receipt := &proto.ReplicaMessage{
		Revision:  record.GetRevision(),
		Heartbeat: f.heartbeatSender.BuildNodeState(),
	}
	if err := stream.Send(receipt); err != nil {
		return err
	}

	f.heartbeatSender.MarkReceiptSent()
	return nil
}

// handleCommit advances the local committed revision and notifies the
// watch subsystem so buffered records up to this revision can be delivered.
func (f *Follower) handleCommit(committedRevision int64) {
	f.state.SetCommitted(committedRevision)

	if f.watchNotifier != nil {
		f.watchNotifier.AdvanceCommittedRevision(committedRevision)
	}
}

// handleCompact updates the local compaction revision to match the
// Primary's compaction point.
func (f *Follower) handleCompact(compactionRevision int64) {
	f.state.SetCompaction(compactionRevision)

	f.logger.Info("received compaction revision update",
		"compaction_revision", compactionRevision,
	)
}
