// Copyright 2026 Nadrama Pty Ltd
// SPDX-License-Identifier: Apache-2.0

package primary

import (
	"testing"
	"time"

	"github.com/nadrama-com/netsy/internal/nodestate"
)

func TestReplicasAddAndGet(t *testing.T) {
	m := NewReplicas()

	entry := m.Add("node-a")
	if entry.NodeID != "node-a" {
		t.Fatalf("expected node-a, got %s", entry.NodeID)
	}

	got, ok := m.Get("node-a")
	if !ok {
		t.Fatal("expected node-a to be found")
	}
	if got.NodeID != "node-a" {
		t.Fatalf("expected node-a, got %s", got.NodeID)
	}
}

func TestReplicasRemove(t *testing.T) {
	m := NewReplicas()
	m.Add("node-a")
	m.Remove("node-a")

	_, ok := m.Get("node-a")
	if ok {
		t.Fatal("expected node-a to be removed")
	}
}

func TestReplicasUpdateHeartbeat(t *testing.T) {
	m := NewReplicas()
	m.Add("node-a")

	ok := m.UpdateHeartbeat("node-a", nodestate.HealthHealthy, nodestate.PrimaryReplica, 42)
	if !ok {
		t.Fatal("expected update to succeed")
	}

	entry, _ := m.Get("node-a")
	if entry.Health() != nodestate.HealthHealthy {
		t.Fatalf("expected healthy, got %s", entry.Health())
	}
	if entry.LatestRevision.Load() != 42 {
		t.Fatalf("expected revision 42, got %d", entry.LatestRevision.Load())
	}
}

func TestReplicasUpdateHeartbeatNotFound(t *testing.T) {
	m := NewReplicas()

	ok := m.UpdateHeartbeat("node-x", nodestate.HealthHealthy, nodestate.PrimaryReplica, 1)
	if ok {
		t.Fatal("expected update to fail for unknown node")
	}
}

func TestReplicasAll(t *testing.T) {
	m := NewReplicas()
	m.Add("node-a")
	m.Add("node-b")

	all := m.All()
	if len(all) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(all))
	}
}

func TestReplicasReset(t *testing.T) {
	m := NewReplicas()
	m.Add("node-a")
	m.Add("node-b")
	m.Reset()

	all := m.All()
	if len(all) != 0 {
		t.Fatalf("expected 0 entries after reset, got %d", len(all))
	}
}

func TestReplicasHeartbeatTimestamp(t *testing.T) {
	m := NewReplicas()
	entry := m.Add("node-a")

	before := time.Now().UnixNano()
	time.Sleep(time.Millisecond)
	m.UpdateHeartbeat("node-a", nodestate.HealthHealthy, nodestate.PrimaryReplica, 1)
	after := time.Now().UnixNano()

	lastHB := entry.LastHeartbeat.Load()
	if lastHB < before || lastHB > after {
		t.Fatalf("heartbeat timestamp %d not in expected range [%d, %d]", lastHB, before, after)
	}
}
