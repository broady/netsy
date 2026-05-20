// SPDX-License-Identifier: Apache-2.0

package elector

import (
	"log/slog"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/netsy-dev/netsy/internal/nodestate"
	"github.com/netsy-dev/netsy/internal/storage"
)

// countStackGoroutines returns the number of goroutines whose stack
// contains substr.
func countStackGoroutines(substr string) int {
	buf := make([]byte, 1<<20)
	n := runtime.Stack(buf, true)
	return strings.Count(string(buf[:n]), substr)
}

// TestOnLoseLeadershipWaitsForGoroutines verifies that onLoseLeadership
// waits for all spawned goroutines to exit before proceeding to
// nodeMap.Reset() and state mutations. With the WaitGroup fix,
// goroutines are guaranteed to have stopped when onLoseLeadership
// returns.
func TestOnLoseLeadershipWaitsForGoroutines(t *testing.T) {
	t.Parallel()

	state := nodestate.New(slog.Default())
	runner := &Runner{
		logger:   slog.Default(),
		nodeID:   "node-a",
		peerAddr: "127.0.0.1:2381",
		state:    state,
		server: NewServer(
			slog.Default(),
			"cluster-a",
			storage.NewMemoryStore(),
			state,
			time.Second,   // heartbeatInterval (nonzero to start health loop)
			0,             // deregTimeout
			0,             // degradationCount
			"node-a",
			1,
			nil,
			0,
			0,
			nil,
			nil,
			nil,
		),
	}

	// Acquire leadership -- spawns goroutines (bootstrap, health loop)
	if err := runner.onAcquireLeadership(); err != nil {
		t.Fatalf("onAcquireLeadership() error = %v", err)
	}

	// Let goroutines start
	time.Sleep(50 * time.Millisecond)
	healthBefore := countStackGoroutines("runHealthCheckLoop")
	if healthBefore == 0 {
		t.Fatal("health check loop goroutine not running after onAcquireLeadership")
	}

	// Lose leadership -- should cancel and wait for all goroutines
	if err := runner.onLoseLeadership(); err != nil {
		t.Fatalf("onLoseLeadership() error = %v", err)
	}

	// With the WaitGroup fix, onLoseLeadership blocks until all
	// goroutines have exited. Give a tiny grace period for stack
	// cleanup, then verify no health-check goroutines remain.
	time.Sleep(10 * time.Millisecond)
	healthAfter := countStackGoroutines("runHealthCheckLoop")
	if healthAfter > 0 {
		t.Fatalf("health check loop goroutine still running after onLoseLeadership (count=%d)", healthAfter)
	}

	// nodeMap was reset only after goroutines stopped.
	if runner.server.nodeMap.Ready() {
		t.Fatal("expected nodeMap.Ready()=false after onLoseLeadership")
	}
}
