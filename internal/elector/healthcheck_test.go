// Copyright 2026 Nadrama Pty Ltd
// SPDX-License-Identifier: Apache-2.0

package elector

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/nadrama-com/netsy/internal/nodestate"
	"github.com/nadrama-com/netsy/internal/storage"
)

func newTestServer(heartbeatInterval time.Duration, degradationCount int) *Server {
	state := nodestate.New(slog.Default())
	_ = state.SetElector(nodestate.ElectorLeader)
	return NewServer(
		slog.Default(),
		"test-cluster",
		storage.NewMemoryStore(),
		state,
		0, // deregTimeout
		heartbeatInterval,
		degradationCount,
	)
}

func TestCheckNodeHealthMarksStaleNode(t *testing.T) {
	srv := newTestServer(50*time.Millisecond, 2)
	srv.nodeMap.SetReady()

	srv.nodeMap.Add(NodeEntry{
		NodeID:        "node-a",
		MemberID:      1,
		LastHeartbeat: time.Now().Add(-200 * time.Millisecond),
		HealthState:   nodestate.HealthHealthy,
	})

	srv.checkNodeHealth(context.Background())

	entry, ok := srv.nodeMap.Get("node-a")
	if !ok {
		t.Fatal("expected node-a to exist")
	}
	if entry.HealthState != nodestate.HealthDegraded {
		t.Fatalf("expected degraded, got %s", entry.HealthState)
	}
}

func TestCheckNodeHealthSkipsFreshNode(t *testing.T) {
	srv := newTestServer(50*time.Millisecond, 2)
	srv.nodeMap.SetReady()

	srv.nodeMap.Add(NodeEntry{
		NodeID:        "node-a",
		MemberID:      1,
		LastHeartbeat: time.Now(),
		HealthState:   nodestate.HealthHealthy,
	})

	srv.checkNodeHealth(context.Background())

	entry, ok := srv.nodeMap.Get("node-a")
	if !ok {
		t.Fatal("expected node-a to exist")
	}
	if entry.HealthState != nodestate.HealthHealthy {
		t.Fatalf("expected healthy, got %s", entry.HealthState)
	}
}

func TestCheckNodeHealthSkipsAlreadyDegraded(t *testing.T) {
	srv := newTestServer(50*time.Millisecond, 2)
	srv.nodeMap.SetReady()

	srv.nodeMap.Add(NodeEntry{
		NodeID:        "node-a",
		MemberID:      1,
		LastHeartbeat: time.Now().Add(-time.Hour),
		HealthState:   nodestate.HealthDegraded,
	})

	srv.checkNodeHealth(context.Background())

	entry, ok := srv.nodeMap.Get("node-a")
	if !ok {
		t.Fatal("expected node-a to exist")
	}
	if entry.HealthState != nodestate.HealthDegraded {
		t.Fatalf("expected degraded, got %s", entry.HealthState)
	}
}

func TestCheckNodeHealthNotReadySkips(t *testing.T) {
	srv := newTestServer(50*time.Millisecond, 2)

	srv.nodeMap.Add(NodeEntry{
		NodeID:        "node-a",
		MemberID:      1,
		LastHeartbeat: time.Now().Add(-time.Hour),
		HealthState:   nodestate.HealthHealthy,
	})

	srv.checkNodeHealth(context.Background())

	entry, ok := srv.nodeMap.Get("node-a")
	if !ok {
		t.Fatal("expected node-a to exist")
	}
	if entry.HealthState != nodestate.HealthHealthy {
		t.Fatalf("expected healthy (not ready, should skip), got %s", entry.HealthState)
	}
}

func TestCheckNodeHealthDeregistersAfterTimeout(t *testing.T) {
	state := nodestate.New(slog.Default())
	_ = state.SetElector(nodestate.ElectorLeader)
	srv := NewServer(
		slog.Default(),
		"test-cluster",
		storage.NewMemoryStore(),
		state,
		100*time.Millisecond, // deregTimeout
		50*time.Millisecond,
		2,
	)
	srv.nodeMap.SetReady()

	srv.nodeMap.Add(NodeEntry{
		NodeID:        "node-a",
		MemberID:      1,
		LastHeartbeat: time.Now().Add(-time.Hour),
		DegradedAt:    time.Now().Add(-time.Hour),
		HealthState:   nodestate.HealthDegraded,
	})

	srv.checkNodeHealth(context.Background())

	_, ok := srv.nodeMap.Get("node-a")
	if ok {
		t.Fatal("expected node-a to be deregistered")
	}
}

func TestCheckNodeHealthKeepsDegradedBeforeTimeout(t *testing.T) {
	state := nodestate.New(slog.Default())
	_ = state.SetElector(nodestate.ElectorLeader)
	srv := NewServer(
		slog.Default(),
		"test-cluster",
		storage.NewMemoryStore(),
		state,
		time.Hour, // deregTimeout — far in the future
		50*time.Millisecond,
		2,
	)
	srv.nodeMap.SetReady()

	srv.nodeMap.Add(NodeEntry{
		NodeID:        "node-a",
		MemberID:      1,
		LastHeartbeat: time.Now().Add(-200 * time.Millisecond),
		DegradedAt:    time.Now(), // just became degraded
		HealthState:   nodestate.HealthDegraded,
	})

	srv.checkNodeHealth(context.Background())

	entry, ok := srv.nodeMap.Get("node-a")
	if !ok {
		t.Fatal("expected node-a to still exist")
	}
	if entry.HealthState != nodestate.HealthDegraded {
		t.Fatalf("expected degraded, got %s", entry.HealthState)
	}
}
