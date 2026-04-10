// Copyright 2026 Nadrama Pty Ltd
// SPDX-License-Identifier: Apache-2.0

package elector

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/nadrama-com/s3lect"

	"github.com/nadrama-com/netsy/internal/config"
	"github.com/nadrama-com/netsy/internal/nodestate"
	"github.com/nadrama-com/netsy/internal/storage"
)

// lockfilePath is the object storage key used by s3lect for Elector coordination.
const lockfilePath = "leader/elector.json"

// Runner manages the s3lect Elector, the dedicated HTTPS election health
// server, and wires leadership change notifications into the local node state.
type Runner struct {
	logger       *slog.Logger
	nodeID       string
	peerAddr     string
	elector      s3lect.Elector
	healthSrv    *s3lect.HealthServer
	state        *nodestate.State
	server       *Server
	leaderCancel context.CancelFunc // cancels bootstrap and deregistration loop on leadership loss
}

// New creates a Runner that is ready to start. It configures the s3lect
// Elector with peer-mode enabled and registers leadership callbacks that
// update the local node state.
func New(
	logger *slog.Logger,
	cfg *config.Config,
	state *nodestate.State,
	store storage.ObjectStorage,
	serverCert *tls.Certificate,
) (*Runner, error) {
	caCertPEM, err := os.ReadFile(cfg.TLSCACert)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA cert for s3lect peer mode: %w", err)
	}

	r := &Runner{
		logger:   logger,
		nodeID:   cfg.NodeID,
		peerAddr: cfg.AdvertisePeer,
		state:    state,
	}

	electorCfg := &s3lect.ElectorConfig{
		LockfilePath:       lockfilePath,
		ServerID:           cfg.NodeID,
		ServerAddr:         cfg.AdvertiseElection,
		FrequentInterval:   5 * time.Second,
		InfrequentInterval: 30 * time.Second,
		LeaderTimeout:      15 * time.Second,
		PeerMode:           true,
		PeerHealthPath:     "/health/leadership",
		PeerCACert:         caCertPEM,
		OnAcquireLeadership: func(ctx context.Context) error {
			return r.onAcquireLeadership()
		},
		OnLoseLeadership: func(ctx context.Context) error {
			return r.onLoseLeadership()
		},
	}

	elector, err := s3lect.NewS3Elector(s3lect.S3ElectorOptions{
		Config:  electorCfg,
		Storage: store,
		Logger:  logger.With("component", "s3lect"),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create s3lect elector: %w", err)
	}
	r.elector = elector

	healthSrv, err := s3lect.NewHealthServer(s3lect.HealthServerConfig{
		BindAddress: cfg.BindElection,
		Certificate: *serverCert,
		Elector:     elector,
		Logger:      logger.With("component", "s3lect-health"),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create s3lect health server: %w", err)
	}
	r.healthSrv = healthSrv

	r.server = NewServer(
		logger.With("component", "elector-server"),
		cfg.ClusterID,
		store,
		state,
		cfg.Elector.DeregistrationTimeout.Duration,
		cfg.Elector.HeartbeatInterval.Duration,
		cfg.Elector.DegradationCount,
	)

	return r, nil
}

// Start begins the election health server and the s3lect election loop.
func (r *Runner) Start(ctx context.Context) error {
	if err := r.healthSrv.Start(ctx); err != nil {
		return fmt.Errorf("failed to start election health server: %w", err)
	}
	r.logger.Info("election health server started")

	if err := r.elector.Start(ctx); err != nil {
		return fmt.Errorf("failed to start s3lect elector: %w", err)
	}
	r.logger.Info("s3lect elector started")
	return nil
}

// WaitForFirstElection blocks until the first election cycle completes,
// then updates ClusterState with the current Elector.
func (r *Runner) WaitForFirstElection(ctx context.Context) (*s3lect.LeadershipStatus, error) {
	status, err := r.elector.WaitForNextElection(ctx, time.Time{})
	if err != nil {
		return nil, err
	}
	r.updateClusterElector(status)
	return status, nil
}

// IsLeader reports whether this node is currently the Elector.
func (r *Runner) IsLeader() bool {
	return r.elector.IsLeader()
}

// LeaderID returns the node ID of the current Elector.
func (r *Runner) LeaderID() string {
	return r.elector.LeaderID()
}

// LeaderAddr returns the advertise address of the current Elector.
func (r *Runner) LeaderAddr() string {
	status := r.elector.GetLeadershipStatus()
	if status == nil {
		return ""
	}
	return status.LeaderAddr
}

// ElectorServer returns the Elector gRPC server for registration with a
// gRPC server externally.
func (r *Runner) ElectorServer() *Server {
	return r.server
}

// Stop gracefully stops the s3lect elector and health server. The elector
// resigns leadership if currently held.
func (r *Runner) Stop(ctx context.Context) {
	if err := r.elector.Stop(); err != nil {
		r.logger.Error("error stopping s3lect elector", "error", err)
	}
	if err := r.healthSrv.Stop(ctx); err != nil {
		r.logger.Error("error stopping election health server", "error", err)
	}
}

func (r *Runner) onAcquireLeadership() error {
	r.logger.Info("acquired elector leadership")
	if err := r.state.SetElector(nodestate.ElectorLeader); err != nil {
		r.logger.Error("failed to transition elector state to leader", "error", err)
		return err
	}
	r.state.SetClusterElector(nodestate.NodeInfo{
		NodeID:            r.nodeID,
		PeerAdvertiseAddr: r.peerAddr,
	})

	ctx, cancel := context.WithCancel(context.Background())
	r.leaderCancel = cancel

	go func() {
		if err := r.server.Bootstrap(ctx); err != nil {
			r.logger.Error("elector bootstrap failed", "error", err)
		}
	}()
	go r.server.runHealthCheckLoop(ctx)

	return nil
}

func (r *Runner) onLoseLeadership() error {
	r.logger.Info("lost elector leadership")
	if r.leaderCancel != nil {
		r.leaderCancel()
		r.leaderCancel = nil
	}
	r.server.nodeMap.Reset()
	if err := r.state.SetElector(nodestate.ElectorFollower); err != nil {
		r.logger.Error("failed to transition elector state to follower", "error", err)
	}
	return nil
}

// updateClusterElector sets the Elector in ClusterState from s3lect status.
func (r *Runner) updateClusterElector(status *s3lect.LeadershipStatus) {
	if status == nil {
		return
	}
	r.state.SetClusterElector(nodestate.NodeInfo{
		NodeID:            status.LeaderID,
		PeerAdvertiseAddr: status.LeaderAddr,
	})
}
