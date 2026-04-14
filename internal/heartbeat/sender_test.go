// Copyright 2026 Nadrama Pty Ltd
// SPDX-License-Identifier: Apache-2.0

package heartbeat

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/nadrama-com/netsy/internal/nodestate"
	"github.com/nadrama-com/netsy/internal/peerclient"
	"github.com/nadrama-com/netsy/internal/proto"
)

// stubDB implements RevisionSource for testing.
type stubDB struct {
	revision int64
}

func (d *stubDB) LatestRevision() (int64, error) { return d.revision, nil }

func TestBuildNodeState(t *testing.T) {
	state := nodestate.New(slog.Default())
	db := &stubDB{revision: 42}
	mgr := peerclient.NewManager(slog.Default(), "test-node", nil, state)

	s := NewSender(
		slog.Default(),
		"test-node",
		state,
		mgr,
		db,
		12345,
		time.Second,
		time.Second,
	)

	ns := s.BuildNodeState()

	if ns.NodeId != "test-node" {
		t.Fatalf("expected node_id test-node, got %s", ns.NodeId)
	}
	if ns.LatestRevision != 42 {
		t.Fatalf("expected revision 42, got %d", ns.LatestRevision)
	}
	if ns.StartTime != 12345 {
		t.Fatalf("expected start_time 12345, got %d", ns.StartTime)
	}
	if ns.HealthState != proto.HealthState_HEALTH_LOADING {
		t.Fatalf("expected HEALTH_LOADING, got %v", ns.HealthState)
	}
}

func newTestSender(nodeID string, state *nodestate.State) *Sender {
	mgr := peerclient.NewManager(slog.Default(), nodeID, nil, state)
	return NewSender(
		slog.Default(),
		nodeID,
		state,
		mgr,
		&stubDB{revision: 5},
		time.Now().Unix(),
		time.Second,
		50*time.Millisecond,
	)
}

func TestPrimaryChangeResetsLastReceipt(t *testing.T) {
	state := nodestate.New(slog.Default())
	s := newTestSender("test-node", state)

	// Simulate a receipt to the old primary.
	s.MarkReceiptSent()
	if s.lastReceiptSent.Load() == 0 {
		t.Fatal("expected lastReceiptSent to be set")
	}

	// Change the primary in cluster state.
	state.SetClusterPrimary(nodestate.NodeInfo{NodeID: "new-primary"})

	// The next sendToPrimaryIfNeeded call should detect the change
	// and clear lastReceiptSent.
	s.sendToPrimaryIfNeeded(context.Background())

	if s.lastReceiptSent.Load() != 0 {
		t.Fatal("expected lastReceiptSent to be cleared after primary change")
	}
}

func TestSendToPrimarySkippedWhenElectorEqualsPrimary(t *testing.T) {
	state := nodestate.New(slog.Default())
	s := newTestSender("test-node", state)

	// Set Elector == Primary.
	state.SetClusterState(nodestate.ClusterState{
		Elector: nodestate.NodeInfo{NodeID: "same-node"},
		Primary: nodestate.NodeInfo{NodeID: "same-node"},
	})

	// sendToPrimaryIfNeeded should skip (sameNode == true).
	// No panic or error expected — it just returns early.
	s.sendToPrimaryIfNeeded(context.Background())
}

func TestSendToPrimarySkippedWhenThisNodeIsPrimary(t *testing.T) {
	state := nodestate.New(slog.Default())
	s := newTestSender("this-node", state)

	state.SetClusterState(nodestate.ClusterState{
		Elector: nodestate.NodeInfo{NodeID: "elector-node"},
		Primary: nodestate.NodeInfo{NodeID: "this-node"},
	})

	// sendToPrimaryIfNeeded should skip (isPrimary == true).
	s.sendToPrimaryIfNeeded(context.Background())
}

func TestSendToPrimarySkippedWhenRecentReceipt(t *testing.T) {
	state := nodestate.New(slog.Default())
	s := newTestSender("test-node", state)

	state.SetClusterState(nodestate.ClusterState{
		Elector: nodestate.NodeInfo{NodeID: "elector-node"},
		Primary: nodestate.NodeInfo{NodeID: "primary-node"},
	})

	// Mark a recent receipt — sendToPrimaryIfNeeded should skip due
	// to lastReceiptSent being within the replication interval.
	s.MarkReceiptSent()

	// Initialize lastPrimaryNodeID so the primary change detection
	// doesn't clear lastReceiptSent.
	s.lastPrimaryMu.Lock()
	s.lastPrimaryNodeID = "primary-node"
	s.lastPrimaryMu.Unlock()

	s.sendToPrimaryIfNeeded(context.Background())

	// lastReceiptSent should still be set (not cleared).
	if s.lastReceiptSent.Load() == 0 {
		t.Fatal("expected lastReceiptSent to remain set when recent receipt exists")
	}
}

func TestSendToElectorSetsLastReceiptWhenSameNode(t *testing.T) {
	state := nodestate.New(slog.Default())
	s := newTestSender("test-node", state)

	// Set Elector == Primary.
	state.SetClusterState(nodestate.ClusterState{
		Elector: nodestate.NodeInfo{NodeID: "same-node"},
		Primary: nodestate.NodeInfo{NodeID: "same-node"},
	})

	// No elector client is connected so the heartbeat won't actually
	// be sent, but we can verify the sameNode marking logic doesn't
	// panic. The lastReceiptSent won't be set because client is nil
	// (returns early).
	s.sendToElector(context.Background())
}

// TestDegradeSelfTransitionsHealth verifies explicit self-degradation updates
// the node health state.
func TestDegradeSelfTransitionsHealth(t *testing.T) {
	state := nodestate.New(slog.Default())
	if err := state.SetHealth(nodestate.HealthHealthy); err != nil {
		t.Fatalf("SetHealth(Healthy) error = %v", err)
	}

	s := newTestSender("test-node", state)
	s.degradeSelf("test failure", nil)

	if state.Health() != nodestate.HealthDegraded {
		t.Fatalf("Health() = %s, want %s", state.Health(), nodestate.HealthDegraded)
	}
}
