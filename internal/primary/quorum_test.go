// Netsy <https://netsy.dev>
// Copyright 2026 Nadrama Pty Ltd
// SPDX-License-Identifier: Apache-2.0

package primary

import (
	"testing"
	"time"

	"github.com/nadrama-com/netsy/internal/nodestate"
)

func TestRequiredReceipts(t *testing.T) {
	tests := []struct {
		name      string
		quorum    int
		nodeCount int
		want      int
	}{
		{"disabled", 0, 5, 0},
		{"static_2", 2, 5, 2},
		{"static_1", 1, 3, 1},
		{"majority_7_nodes", -1, 7, 3},
		{"majority_5_nodes", -1, 5, 2},
		{"majority_4_nodes", -1, 4, 2},
		{"majority_3_nodes", -1, 3, 1},
		{"majority_2_nodes", -1, 2, 1},
		{"majority_1_node", -1, 1, 0},
		{"majority_0_nodes", -1, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := requiredReceipts(tt.quorum, tt.nodeCount)
			if got != tt.want {
				t.Fatalf("requiredReceipts(%d, %d) = %d, want %d",
					tt.quorum, tt.nodeCount, got, tt.want)
			}
		})
	}
}

func TestSelectTxnPath(t *testing.T) {
	tests := []struct {
		name             string
		quorum           int
		nodeCount        int
		healthyForQuorum int
		wantQuorum       bool
		wantRequired     int
	}{
		{"disabled_quorum", 0, 3, 2, false, 0},
		{"majority_met", -1, 3, 1, true, 1},
		{"majority_not_met", -1, 3, 0, false, 0},
		{"static_met", 2, 5, 2, true, 2},
		{"static_not_met", 2, 5, 1, false, 0},
		{"single_node_majority", -1, 1, 0, false, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := selectTxnStrategy(tt.quorum, tt.nodeCount, tt.healthyForQuorum)
			if s.useQuorum != tt.wantQuorum {
				t.Fatalf("useQuorum = %v, want %v", s.useQuorum, tt.wantQuorum)
			}
			if s.useQuorum && s.requiredReceipts != tt.wantRequired {
				t.Fatalf("requiredReceipts = %d, want %d", s.requiredReceipts, tt.wantRequired)
			}
		})
	}
}

func TestReceiptCollectorCompletesOnThreshold(t *testing.T) {
	c := newReceiptCollector(42, 2)

	c.collectReceipt("node-a")
	if c.isComplete() {
		t.Fatal("should not be complete with 1 receipt")
	}

	c.collectReceipt("node-b")
	if !c.isComplete() {
		t.Fatal("should be complete with 2 receipts")
	}

	ok := c.wait(time.Millisecond)
	if !ok {
		t.Fatal("wait should return true when quorum met")
	}
}

func TestReceiptCollectorDeduplicatesReceipts(t *testing.T) {
	c := newReceiptCollector(42, 2)

	c.collectReceipt("node-a")
	c.collectReceipt("node-a")
	if c.isComplete() {
		t.Fatal("duplicate receipt should not count twice")
	}

	c.collectReceipt("node-b")
	if !c.isComplete() {
		t.Fatal("should be complete with 2 distinct receipts")
	}
}

func TestReceiptCollectorTimeout(t *testing.T) {
	c := newReceiptCollector(42, 2)

	c.collectReceipt("node-a")
	ok := c.wait(10 * time.Millisecond)
	if ok {
		t.Fatal("wait should return false on timeout")
	}
}

func TestReceiptCollectorUnackedQuorumNodeIDs(t *testing.T) {
	c := newReceiptCollector(42, 2)

	// node-a: acked
	// node-b: healthy + receipted = eligible, not acked → should appear
	// node-c: healthy + receipted = eligible, not acked → should appear
	// node-d: degraded + receipted = not eligible → should NOT appear
	// node-e: healthy + no prior receipt = not eligible → should NOT appear
	replicaA := &Replica{NodeID: "node-a"}
	replicaA.SetHealth(nodestate.HealthHealthy)
	replicaA.ReceiptCount.Store(1)

	replicaB := &Replica{NodeID: "node-b"}
	replicaB.SetHealth(nodestate.HealthHealthy)
	replicaB.ReceiptCount.Store(1)

	replicaC := &Replica{NodeID: "node-c"}
	replicaC.SetHealth(nodestate.HealthHealthy)
	replicaC.ReceiptCount.Store(3)

	replicaD := &Replica{NodeID: "node-d"}
	replicaD.SetHealth(nodestate.HealthDegraded)
	replicaD.ReceiptCount.Store(2)

	replicaE := &Replica{NodeID: "node-e"}
	replicaE.SetHealth(nodestate.HealthHealthy)

	replicas := []*Replica{replicaA, replicaB, replicaC, replicaD, replicaE}
	c.collectReceipt("node-a")

	missing := c.unackedQuorumNodeIDs(replicas)
	if len(missing) != 2 {
		t.Fatalf("unackedQuorumNodeIDs = %v, want 2 entries", missing)
	}

	found := make(map[string]bool)
	for _, id := range missing {
		found[id] = true
	}
	if !found["node-b"] || !found["node-c"] {
		t.Fatalf("unackedQuorumNodeIDs = %v, want node-b and node-c", missing)
	}
}

func TestReceiptCollectorIgnoresReceiptAfterDone(t *testing.T) {
	c := newReceiptCollector(42, 1)

	c.collectReceipt("node-a")
	if !c.isComplete() {
		t.Fatal("should be complete")
	}

	// Should not panic or change state.
	c.collectReceipt("node-b")
	if !c.isComplete() {
		t.Fatal("should still be complete")
	}
}
