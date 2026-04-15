// Copyright 2026 Nadrama Pty Ltd
// SPDX-License-Identifier: Apache-2.0

package primary

import (
	"context"
	"time"

	"github.com/nadrama-com/netsy/internal/discovery"
)

// RunCompactionScheduler runs a periodic loop that queries all registered
// nodes for their minimum watch revision and computes the cluster-wide
// global minimum.
// TODO: The actual compaction notice/confirmation protocol is
// implemented in Phase 21; this scheduler only determines whether a new
// compaction revision is available and logs the result.
func (s *Server) RunCompactionScheduler(ctx context.Context) {
	interval := s.config.CompactionInterval.Duration
	if interval <= 0 {
		s.logger.Info("compaction scheduling disabled (compaction_interval not set)")
		return
	}

	s.logger.Info("compaction scheduler started", "interval", interval)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("compaction scheduler stopping")
			return
		case <-ticker.C:
			s.runCompactionCycle(ctx)
		}
	}
}

// runCompactionCycle performs a single compaction scheduling pass. It
// queries every registered node for its minimum watch revision, computes
// the global minimum, and logs whether a new compaction revision is
// available. If any node cannot be reached, the cycle is aborted.
func (s *Server) runCompactionCycle(ctx context.Context) {
	if err := s.requireActivePrimary(); err != nil {
		return
	}

	registrations, err := discovery.ListNodeRegistrations(ctx, s.storageClient)
	if err != nil {
		s.logger.Warn("compaction cycle: failed to list node registrations", "error", err)
		return
	}

	if len(registrations) == 0 {
		s.logger.Debug("compaction cycle: no registered nodes found")
		return
	}

	var globalMin int64 = -1

	for _, reg := range registrations {
		addr := reg.PeerAdvertiseAddress
		if addr == "" {
			s.logger.Warn("compaction cycle: node has no peer advertise address",
				"node_id", reg.NodeID,
			)
			return
		}

		queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		minRev, err := s.peerClients.GetMinWatchRevision(queryCtx, addr)
		cancel()

		if err != nil {
			s.logger.Warn("compaction cycle: failed to query node, aborting cycle",
				"node_id", reg.NodeID,
				"error", err,
			)
			return
		}

		if globalMin < 0 || minRev < globalMin {
			globalMin = minRev
		}
	}

	if globalMin <= 0 {
		s.logger.Debug("compaction cycle: global minimum revision is zero, skipping")
		return
	}

	currentCompaction := s.state.Compaction()
	if globalMin <= currentCompaction {
		s.logger.Debug("compaction cycle: no advancement needed",
			"global_min", globalMin,
			"current_compaction", currentCompaction,
		)
		return
	}

	s.logger.Info("compaction cycle: new compaction revision available",
		"proposed_revision", globalMin,
		"current_compaction", currentCompaction,
		"nodes_queried", len(registrations),
	)

	// TODO: Phase 21 will implement the compaction notice/confirmation protocol
	// here. For now, only the revision is logged.
}
