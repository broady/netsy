// Copyright 2026 Nadrama Pty Ltd
// SPDX-License-Identifier: Apache-2.0

package primary

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/nadrama-com/netsy/internal/nodestate"
	"github.com/nadrama-com/netsy/internal/proto"
)

func newTestServer(t *testing.T, state *nodestate.State, heartbeatInterval time.Duration, degradationCount int) *Server {
	t.Helper()
	return &Server{
		logger:            slog.Default(),
		state:             state,
		replicas:          NewReplicas(),
		heartbeatInterval: heartbeatInterval,
		degradationCount:  degradationCount,
	}
}

func TestSendHeartbeatRequiresPrimary(t *testing.T) {
	state := nodestate.New(slog.Default())
	srv := newTestServer(t, state, 100*time.Millisecond, 2)

	// State is PrimaryReplica by default, so SendHeartbeat should fail
	_, err := srv.SendHeartbeat(context.Background(), &proto.NodeState{
		NodeId:      "node-a",
		HealthState: proto.HealthState_HEALTH_HEALTHY,
	})
	if err == nil {
		t.Fatal("expected error when node is not primary")
	}
}

func TestSendHeartbeatSuccess(t *testing.T) {
	state := nodestate.New(slog.Default())
	srv := newTestServer(t, state, 100*time.Millisecond, 2)

	if err := state.SetPrimary(nodestate.PrimaryStarting); err != nil {
		t.Fatal(err)
	}

	srv.replicas.Add("node-a")

	_, err := srv.SendHeartbeat(context.Background(), &proto.NodeState{
		NodeId:         "node-a",
		HealthState:    proto.HealthState_HEALTH_HEALTHY,
		PrimaryState:   proto.PrimaryState_PRIMARY_REPLICA,
		LatestRevision: 10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entry, ok := srv.replicas.Get("node-a")
	if !ok {
		t.Fatal("expected node-a in replica map")
	}
	if entry.Health() != nodestate.HealthHealthy {
		t.Fatalf("expected healthy, got %s", entry.Health())
	}
	if entry.LatestRevision.Load() != 10 {
		t.Fatalf("expected revision 10, got %d", entry.LatestRevision.Load())
	}
}

func TestSendHeartbeatNotRegistered(t *testing.T) {
	state := nodestate.New(slog.Default())
	srv := newTestServer(t, state, 100*time.Millisecond, 2)

	if err := state.SetPrimary(nodestate.PrimaryStarting); err != nil {
		t.Fatal(err)
	}

	_, err := srv.SendHeartbeat(context.Background(), &proto.NodeState{
		NodeId:      "node-unknown",
		HealthState: proto.HealthState_HEALTH_HEALTHY,
	})
	if err == nil {
		t.Fatal("expected error for unknown replica")
	}
}

func TestDegradationCheck(t *testing.T) {
	state := nodestate.New(slog.Default())
	srv := newTestServer(t, state, 50*time.Millisecond, 2)

	if err := state.SetPrimary(nodestate.PrimaryStarting); err != nil {
		t.Fatal(err)
	}

	entry := srv.replicas.Add("node-a")
	entry.SetHealth(nodestate.HealthHealthy)
	entry.LastHeartbeat.Store(time.Now().Add(-200 * time.Millisecond).UnixNano())

	srv.checkDegradation()

	if entry.Health() != nodestate.HealthDegraded {
		t.Fatal("expected node-a to be marked degraded")
	}
}

func TestDegradationSkipsAlreadyDegraded(t *testing.T) {
	state := nodestate.New(slog.Default())
	srv := newTestServer(t, state, 50*time.Millisecond, 2)

	entry := srv.replicas.Add("node-a")
	entry.SetHealth(nodestate.HealthDegraded)
	entry.LastHeartbeat.Store(time.Now().Add(-time.Hour).UnixNano())

	srv.checkDegradation()

	if entry.Health() != nodestate.HealthDegraded {
		t.Fatal("expected node-a to remain degraded")
	}
}
