// Copyright 2026 Nadrama Pty Ltd
// SPDX-License-Identifier: Apache-2.0

package heartbeat

import (
	"context"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nadrama-com/netsy/internal/nodestate"
	"github.com/nadrama-com/netsy/internal/proto"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

// stubDB implements RevisionSource for testing.
type stubDB struct {
	revision int64
}

func (d *stubDB) LatestRevision() (int64, error) { return d.revision, nil }

// stubElectorClient records SendHeartbeat calls.
type stubElectorClient struct {
	proto.ElectorClient
	calls atomic.Int64
}

func (c *stubElectorClient) SendHeartbeat(_ context.Context, _ *proto.NodeState, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	c.calls.Add(1)
	return &emptypb.Empty{}, nil
}

// stubPrimaryClient records SendHeartbeat calls.
type stubPrimaryClient struct {
	proto.PrimaryClient
	calls atomic.Int64
}

func (c *stubPrimaryClient) SendHeartbeat(_ context.Context, _ *proto.NodeState, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	c.calls.Add(1)
	return &emptypb.Empty{}, nil
}

func TestSenderSendsToElector(t *testing.T) {
	state := nodestate.New(slog.Default())
	db := &stubDB{revision: 5}
	ec := &stubElectorClient{}

	s := NewSender(
		slog.Default(),
		"test-node",
		state,
		db,
		time.Now().Unix(),
		50*time.Millisecond,
		0,
	)
	s.SetElector("elector-node", ec)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	s.Run(ctx)

	if ec.calls.Load() < 2 {
		t.Fatalf("expected at least 2 elector heartbeats, got %d", ec.calls.Load())
	}
}

func TestSenderSendsToPrimaryWhenNoReceipt(t *testing.T) {
	state := nodestate.New(slog.Default())
	db := &stubDB{revision: 5}
	ec := &stubElectorClient{}
	pc := &stubPrimaryClient{}

	s := NewSender(
		slog.Default(),
		"test-node",
		state,
		db,
		time.Now().Unix(),
		1*time.Second,
		50*time.Millisecond,
	)
	s.SetElector("elector-node", ec)
	s.SetPrimary("primary-node", pc)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	s.Run(ctx)

	if pc.calls.Load() < 2 {
		t.Fatalf("expected at least 2 primary heartbeats, got %d", pc.calls.Load())
	}
}

func TestSenderSkipsPrimaryWhenRecentReceipt(t *testing.T) {
	state := nodestate.New(slog.Default())
	db := &stubDB{revision: 5}
	pc := &stubPrimaryClient{}

	s := NewSender(
		slog.Default(),
		"test-node",
		state,
		db,
		time.Now().Unix(),
		1*time.Second,
		50*time.Millisecond,
	)
	s.SetPrimary("primary-node", pc)

	// Mark a recent receipt
	s.MarkReceiptSent()

	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()

	s.Run(ctx)

	if pc.calls.Load() > 1 {
		t.Fatalf("expected at most 1 primary heartbeat (first tick skipped), got %d", pc.calls.Load())
	}
}

func TestSenderSingleHeartbeatWhenElectorIsPrimary(t *testing.T) {
	state := nodestate.New(slog.Default())
	db := &stubDB{revision: 5}
	ec := &stubElectorClient{}
	pc := &stubPrimaryClient{}

	s := NewSender(
		slog.Default(),
		"test-node",
		state,
		db,
		time.Now().Unix(),
		50*time.Millisecond,
		50*time.Millisecond,
	)
	s.SetElector("same-node", ec)
	s.SetPrimary("same-node", pc)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	s.Run(ctx)

	// Primary loop should skip all ticks since sameNode is true
	if pc.calls.Load() != 0 {
		t.Fatalf("expected 0 primary heartbeats when elector == primary, got %d", pc.calls.Load())
	}
	if ec.calls.Load() < 2 {
		t.Fatalf("expected at least 2 elector heartbeats, got %d", ec.calls.Load())
	}
}

func TestBuildNodeState(t *testing.T) {
	state := nodestate.New(slog.Default())
	db := &stubDB{revision: 42}

	s := NewSender(
		slog.Default(),
		"test-node",
		state,
		db,
		12345,
		time.Second,
		time.Second,
	)

	ns := s.buildNodeState()

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

func TestSetElectorResetsSameNode(t *testing.T) {
	s := NewSender(slog.Default(), "n", nodestate.New(slog.Default()), &stubDB{}, 0, time.Second, time.Second)

	s.SetPrimary("node-a", &stubPrimaryClient{})
	s.SetElector("node-a", &stubElectorClient{})

	s.mu.RLock()
	same := s.sameNode
	s.mu.RUnlock()

	if !same {
		t.Fatal("expected sameNode=true when elector and primary have the same node ID")
	}
}

func TestSenderSkipsPrimaryWhenThisNodeIsPrimary(t *testing.T) {
	state := nodestate.New(slog.Default())
	db := &stubDB{revision: 5}
	ec := &stubElectorClient{}
	pc := &stubPrimaryClient{}

	s := NewSender(
		slog.Default(),
		"this-node", // local node ID
		state,
		db,
		time.Now().Unix(),
		50*time.Millisecond,
		50*time.Millisecond,
	)
	s.SetElector("elector-node", ec)
	s.SetPrimary("this-node", pc) // this node is the Primary

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	s.Run(ctx)

	if pc.calls.Load() != 0 {
		t.Fatalf("expected 0 primary heartbeats when this node is the primary, got %d", pc.calls.Load())
	}
	if ec.calls.Load() < 2 {
		t.Fatalf("expected at least 2 elector heartbeats, got %d", ec.calls.Load())
	}
}

func TestSetPrimaryResetsSameNode(t *testing.T) {
	s := NewSender(slog.Default(), "n", nodestate.New(slog.Default()), &stubDB{}, 0, time.Second, time.Second)

	s.SetElector("node-a", &stubElectorClient{})
	s.SetPrimary("node-a", &stubPrimaryClient{})

	s.mu.RLock()
	same := s.sameNode
	s.mu.RUnlock()

	if !same {
		t.Fatal("expected sameNode=true when primary set to same node as elector")
	}

	// Change primary to a different node
	s.SetPrimary("node-b", &stubPrimaryClient{})

	s.mu.RLock()
	same = s.sameNode
	s.mu.RUnlock()

	if same {
		t.Fatal("expected sameNode=false after primary changed to a different node")
	}
}
