// Copyright 2026 Nadrama Pty Ltd
// SPDX-License-Identifier: Apache-2.0

package elector

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/nadrama-com/netsy/internal/nodestate"
	"github.com/nadrama-com/netsy/internal/proto"
	"github.com/nadrama-com/netsy/internal/storage"
)

func TestSendHeartbeatUpdatesNodeState(t *testing.T) {
	state := nodestate.New(slog.Default())
	_ = state.SetElector(nodestate.ElectorLeader)

	srv := NewServer(
		slog.Default(),
		"test-cluster",
		storage.NewMemoryStore(),
		state,
		0,
		time.Second,
		2,
		"", 0, nil, 0, 0, nil,
	)
	srv.nodeMap.SetReady()
	srv.nodeMap.Add(NodeEntry{
		NodeID:        "node-a",
		MemberID:      1,
		LastHeartbeat: time.Now().Add(-time.Hour),
		HealthState:   nodestate.HealthLoading,
	})

	_, err := srv.SendHeartbeat(context.Background(), &proto.NodeState{
		NodeId:         "node-a",
		HealthState:    proto.HealthState_HEALTH_HEALTHY,
		PrimaryState:   proto.PrimaryState_PRIMARY_ACTIVE,
		LatestRevision: 42,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entry, ok := srv.nodeMap.Get("node-a")
	if !ok {
		t.Fatal("expected node-a in map")
	}
	if entry.HealthState != nodestate.HealthHealthy {
		t.Fatalf("expected healthy, got %s", entry.HealthState)
	}
	if entry.PrimaryState != nodestate.PrimaryActive {
		t.Fatalf("expected active, got %s", entry.PrimaryState)
	}
	if entry.LatestRevision != 42 {
		t.Fatalf("expected revision 42, got %d", entry.LatestRevision)
	}
}

func TestSendHeartbeatRejectsNonLeader(t *testing.T) {
	state := nodestate.New(slog.Default())
	// state is Follower by default

	srv := NewServer(
		slog.Default(),
		"test-cluster",
		storage.NewMemoryStore(),
		state,
		0,
		time.Second,
		2,
		"", 0, nil, 0, 0, nil,
	)

	_, err := srv.SendHeartbeat(context.Background(), &proto.NodeState{
		NodeId: "node-a",
	})
	if err == nil {
		t.Fatal("expected error when not elector leader")
	}
}

func TestSendHeartbeatRejectsUnknownNode(t *testing.T) {
	state := nodestate.New(slog.Default())
	_ = state.SetElector(nodestate.ElectorLeader)

	srv := NewServer(
		slog.Default(),
		"test-cluster",
		storage.NewMemoryStore(),
		state,
		0,
		time.Second,
		2,
		"", 0, nil, 0, 0, nil,
	)

	_, err := srv.SendHeartbeat(context.Background(), &proto.NodeState{
		NodeId: "node-unknown",
	})
	if err == nil {
		t.Fatal("expected error for unknown node")
	}
}
