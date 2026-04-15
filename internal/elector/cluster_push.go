// Copyright 2026 Nadrama Pty Ltd
// SPDX-License-Identifier: Apache-2.0

package elector

import (
	"context"
	"sync"
	"time"

	"github.com/nadrama-com/netsy/internal/nodestate"
)

// pushTimeout is the RPC timeout for pushing cluster state to a single node.
const pushTimeout = 5 * time.Second

// pushClusterState builds the current authoritative cluster state and
// pushes it to all registered nodes. The Primary is pushed first,
// then all other nodes in parallel. Failures are logged but do not
// block subsequent pushes.
func (s *Server) pushClusterState(ctx context.Context) {
	cs := s.state.ClusterState()
	cs.NodeCount = s.nodeMap.Count()
	protoCS := nodestate.ClusterStateToProto(cs)

	entries := s.nodeMap.All()
	if len(entries) == 0 {
		return
	}

	s.logger.Info("pushing cluster state",
		"elector", cs.Elector.NodeID,
		"primary", cs.Primary.NodeID,
		"node_count", len(entries),
	)

	// Apply cluster state to this node (state transitions, connection
	// updates) before pushing to remote nodes.
	s.peers.ApplyClusterState(ctx, cs)

	// No remote nodes to push to.
	if len(entries) <= 1 {
		return
	}

	// Remove self from the list.
	remote := make([]NodeEntry, 0, len(entries)-1)
	for _, e := range entries {
		if e.NodeID != s.localNodeID {
			remote = append(remote, e)
		}
	}

	push := func(nodeID, addr string) {
		pushCtx, cancel := context.WithTimeout(ctx, pushTimeout)
		defer cancel()

		if err := s.peers.PushClusterStateTo(pushCtx, addr, protoCS); err != nil {
			s.logger.Warn("failed to push cluster state to node",
				"node_id", nodeID,
				"addr", addr,
				"error", err,
			)
			return
		}
		s.logger.Debug("pushed cluster state to node", "node_id", nodeID)
	}

	// Push to the Primary first synchronously.
	for _, e := range remote {
		if e.NodeID == cs.Primary.NodeID {
			push(e.NodeID, e.PeerAdvertiseAddress)
			break
		}
	}

	// Push to all remaining nodes in parallel.
	var wg sync.WaitGroup
	for _, e := range remote {
		if e.NodeID == cs.Primary.NodeID {
			continue
		}
		wg.Add(1)
		go func(nodeID, addr string) {
			defer wg.Done()
			push(nodeID, addr)
		}(e.NodeID, e.PeerAdvertiseAddress)
	}
	wg.Wait()
}
