// Copyright 2026 Nadrama Pty Ltd
// SPDX-License-Identifier: Apache-2.0

package elector

import (
	"context"
	"time"

	"github.com/nadrama-com/netsy/internal/discovery"
	"github.com/nadrama-com/netsy/internal/nodestate"
)

// deregCheckInterval is how often the deregistration loop checks for
// nodes that should be auto-deregistered.
const deregCheckInterval = 10 * time.Second

// RunDeregistrationLoop periodically checks all nodes in the node map and
// auto-deregisters any node whose health is degraded and whose last
// heartbeat exceeds the deregistration timeout. If deregTimeout is 0,
// auto-deregistration is disabled.
func (s *Server) RunDeregistrationLoop(ctx context.Context) {
	if s.deregTimeout == 0 {
		s.logger.Info("auto-deregistration disabled (timeout is 0)")
		return
	}

	ticker := time.NewTicker(deregCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.checkDeregistrations(ctx)
		}
	}
}

// checkDeregistrations iterates over all nodes and removes any that have
// exceeded the deregistration timeout while in a degraded health state.
func (s *Server) checkDeregistrations(ctx context.Context) {
	if !s.nodeMap.Ready() {
		return
	}

	now := time.Now()
	entries := s.nodeMap.All()

	for _, entry := range entries {
		if entry.HealthState != nodestate.HealthDegraded {
			continue
		}
		if now.Sub(entry.LastHeartbeat) < s.deregTimeout {
			continue
		}

		s.logger.Info("auto-deregistering node",
			"node_id", entry.NodeID,
			"last_heartbeat", entry.LastHeartbeat,
			"timeout", s.deregTimeout,
		)

		s.nodeMap.Remove(entry.NodeID)

		if err := discovery.DeleteNodeRegistration(ctx, s.store, entry.NodeID); err != nil {
			s.logger.Warn("failed to delete registration file during auto-deregistration",
				"node_id", entry.NodeID,
				"error", err,
			)
		}
	}
}
