// SPDX-License-Identifier: Apache-2.0

package elector

import (
	"log/slog"
	"sync"
	"testing"

	"github.com/netsy-dev/netsy/internal/nodestate"
)

// TestPreviousPrimaryAtomicAccess verifies that previousPrimary can be
// safely read and written from multiple goroutines using atomic
// Load/Store. With the atomic.Pointer field, this test passes cleanly
// under -race.
func TestPreviousPrimaryAtomicAccess(t *testing.T) {
	t.Parallel()

	state := nodestate.New(slog.Default())
	srv := &Server{
		logger:      slog.Default(),
		state:       state,
		localNodeID: "node-a",
		nodeMap:     NewNodeMap(slog.Default()),
	}

	var wg sync.WaitGroup

	// Writer 1: simulates clearPrimary (health-check goroutine)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			srv.previousPrimary.Store(&nodestate.NodeInfo{
				NodeID:            "node-b",
				PeerAdvertiseAddr: "10.0.0.2:2381",
			})
		}
	}()

	// Writer 2: simulates onLoseLeadership (s3lect callback goroutine)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			srv.previousPrimary.Store(&nodestate.NodeInfo{})
		}
	}()

	// Reader: simulates checkPreviousPrimary (election loop goroutine)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			if pp := srv.previousPrimary.Load(); pp != nil {
				_ = pp.NodeID
			}
		}
	}()

	wg.Wait()
}
