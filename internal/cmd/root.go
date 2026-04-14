// Copyright 2026 Nadrama Pty Ltd
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nadrama-com/netsy/internal/bootstrap"
	"github.com/nadrama-com/netsy/internal/buildvars"
	"github.com/nadrama-com/netsy/internal/clientapi"
	"github.com/nadrama-com/netsy/internal/config"
	"github.com/nadrama-com/netsy/internal/discovery"
	"github.com/nadrama-com/netsy/internal/elector"
	"github.com/nadrama-com/netsy/internal/healthserver"
	"github.com/nadrama-com/netsy/internal/heartbeat"
	"github.com/nadrama-com/netsy/internal/localdb"
	"github.com/nadrama-com/netsy/internal/mtls"
	"github.com/nadrama-com/netsy/internal/node"
	"github.com/nadrama-com/netsy/internal/nodestate"
	"github.com/nadrama-com/netsy/internal/peerclient"
	"github.com/nadrama-com/netsy/internal/primary"
	"github.com/nadrama-com/netsy/internal/proto"
	"github.com/nadrama-com/netsy/internal/replication"
	"github.com/nadrama-com/netsy/internal/snapshot"
	"github.com/nadrama-com/netsy/internal/storage"
	"github.com/spf13/cobra"
	"go.etcd.io/etcd/server/v3/embed"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
)

var (
	flagConfig   string
	flagValidate string
	flagVerbose  bool
	flagVersion  bool
)

var rootCmd = &cobra.Command{
	Use:   "netsy",
	Short: "Netsy",
	Long:  `netsy is an etcd alternative which implements a subset of the etcd API for use with for Kubernetes.`,
}

func init() {
	pflags := rootCmd.PersistentFlags()
	pflags.BoolVarP(&flagVerbose, "verbose", "v", false, "Enable verbose output")
	pflags.BoolVar(&flagVersion, "version", false, "Show version information")
	pflags.StringVar(&flagConfig, "config", "", "Path to JSONC config file (overrides NETSY_CONFIG env var)")
	pflags.StringVar(&flagValidate, "validate", "", "Validate a config file and exit")
}

// NewRootCmd constructs the netsy CLI root command and wires startup behavior into its Run function.
func NewRootCmd() *cobra.Command {
	// Create logger
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		AddSource: true,
		Level:     slog.LevelInfo,
	}))

	// Define root command
	rootCmd.Run = func(cmd *cobra.Command, args []string) {
		var err error

		// Capture start time early for Primary leader election tie-breaking.
		startTime := time.Now().Unix()

		// Register signal handler early so signals during startup are
		// captured in the buffered channel and handled once we reach
		// the select loop.
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

		// Define flags which can be used to override env vars
		flags := config.FlagOverrides{
			ConfigPath: flagConfig,
			Verbose:    flagVerbose,
		}

		// Check for version flag
		if flagVersion {
			fmt.Printf("netsy %s\n", buildvars.BuildVersion())
			if flagVerbose {
				fmt.Printf("build version: %s\n", buildvars.BuildVersion())
				fmt.Printf("build date: %s\n", buildvars.BuildDate())
				fmt.Printf("commit hash: %s\n", buildvars.CommitHash())
				fmt.Printf("commit date: %s\n", buildvars.CommitDate())
				fmt.Printf("commit branch: %s\n", buildvars.CommitBranch())
			}
			return
		}

		// Handle --validate: load + validate config file, print result, exit
		if flagValidate != "" {
			if err := config.ValidateFile(flagValidate, flags); err != nil {
				fmt.Printf("Validation failed: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("Config is valid.")
			return
		}

		// Load and validate config
		c, err := config.Load(flags)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		// Apply log level filtering based on verbose setting
		filteredLogger := logger
		if c.Verbose {
			filteredLogger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
				AddSource: true,
				Level:     slog.LevelDebug,
			}))
		}

		// Log modes
		if c.Verbose {
			fmt.Println("Verbose output ENABLED")
		}

		// Initialise node state (Loading / Follower / Replica)
		state := nodestate.New(filteredLogger)

		// Start HTTP health server
		healthSrv, err := healthserver.New(filteredLogger, c.BindHealth, state)
		if err != nil {
			filteredLogger.Error("Failed to start health server", "error", err)
			os.Exit(1)
		}
		healthSrv.Start()
		defer healthSrv.Close()
		filteredLogger.Info("starting health (http) server...", "addr", c.BindHealth)

		// Load certs and keys
		tlsFiles, err := config.LoadTLSFiles(c)
		if err != nil {
			filteredLogger.Error("Failed to load TLS files", "err", err)
			jitterWaitThenExit(filteredLogger)
		}
		if err := mtls.ValidateLocalNodeCertificates(c, tlsFiles); err != nil {
			filteredLogger.Error("Invalid local TLS certificates", "err", err)
			jitterWaitThenExit(filteredLogger)
		}
		tlsConfigClientAPI, err := mtls.NewServerTLSConfig(c, tlsFiles, mtls.RoleClient)
		if err != nil {
			filteredLogger.Error("Failed to construct client API TLS config", "err", err)
			jitterWaitThenExit(filteredLogger)
		}
		tlsConfigPeerAPI, err := mtls.NewServerTLSConfig(c, tlsFiles, mtls.RolePeer)
		if err != nil {
			filteredLogger.Error("Failed to construct peer API TLS config", "err", err)
			jitterWaitThenExit(filteredLogger)
		}
		tlsConfigPeerClient, err := mtls.NewClientTLSConfig(tlsFiles)
		if err != nil {
			filteredLogger.Error("Failed to construct peer client TLS config", "err", err)
			jitterWaitThenExit(filteredLogger)
		}

		// Create root context for all background services
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Create object storage client
		storageClient, storageCleanup, err := storage.New(c, filteredLogger)
		if err != nil {
			filteredLogger.Error("Failed to create storage client", "error", err)
			os.Exit(1)
		}
		defer storageCleanup()

		// Instantiate database
		db := localdb.New(fmt.Sprintf("%s/db.sqlite3", c.DataDir))
		err = db.Connect()
		if err != nil {
			filteredLogger.Error("db.Connect error", "error", err)
			jitterWaitThenExit(filteredLogger)
		}

		// Construct the snapshot worker early so the Primary server can hold a
		// stable reference to it during bootstrap and later role transitions.
		snapshotWorker := snapshot.NewWorker(filteredLogger, c, db, storageClient)
		defer snapshotWorker.Stop()

		// Construct the Primary server before bootstrap so the same instance can
		// be wired into peer/client services now and activated later on election.
		primarySrv, err := primary.NewServer(
			filteredLogger.With("component", "primary"),
			c,
			db,
			snapshotWorker,
			storageClient,
			state,
			c.Replication.HeartbeatInterval.Duration,
			c.Replication.DegradationCount,
		)
		if err != nil {
			filteredLogger.Error("Failed to create primary server", "error", err)
			os.Exit(1)
		}

		// Create peer manager for outbound connections
		peerManager := peerclient.NewManager(
			filteredLogger.With("component", "peer-manager"),
			c.NodeID,
			tlsConfigPeerClient,
			state,
		)
		defer peerManager.Close()

		// Construct the client API server before bootstrap so its watch
		// notifier can be used by the replication follower, but do not
		// start serving client traffic until bootstrap completes.
		gopts := []grpc.ServerOption{
			grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
				MinTime:             embed.DefaultGRPCKeepAliveMinTime,
				PermitWithoutStream: false,
			}),
			grpc.KeepaliveParams(keepalive.ServerParameters{
				Time:    embed.DefaultGRPCKeepAliveInterval,
				Timeout: embed.DefaultGRPCKeepAliveTimeout,
			}),
		}
		gopts = append(gopts, grpc.Creds(credentials.NewTLS(tlsConfigClientAPI)))
		grpcServer := grpc.NewServer(gopts...)
		clientApiServer := clientapi.NewServer(filteredLogger, c, db, grpcServer, primarySrv, state)

		// Start elector — s3lect election, health server, and elector service.
		// Created after DB and peer manager so all election dependencies are
		// available at construction time.
		electionRunner, err := elector.New(
			filteredLogger, c, state, storageClient, tlsFiles.ServerCert,
			startTime, db, peerManager,
		)
		if err != nil {
			filteredLogger.Error("Failed to create election runner", "error", err)
			jitterWaitThenExit(filteredLogger)
		}
		if err := electionRunner.Start(ctx); err != nil {
			filteredLogger.Error("Failed to start election runner", "error", err)
			jitterWaitThenExit(filteredLogger)
		}
		defer electionRunner.Stop(ctx)
		filteredLogger.Info("starting election (https) server...", "addr", c.BindElection)

		// Wait for first election cycle to know the current Elector
		electionStatus, err := electionRunner.WaitForFirstElection(ctx)
		if err != nil {
			filteredLogger.Error("Failed to complete first elector leader election cycle", "error", err)
			jitterWaitThenExit(filteredLogger)
		}
		filteredLogger.Info("first elector leader election cycle complete",
			"is_leader", electionStatus.IsLeader,
			"leader_id", electionStatus.LeaderID,
			"leader_addr", electionStatus.LeaderAddr,
		)

		// s3lect returns the Elector's election-health address; peer RPCs must
		// use the node's peer advertise address from service discovery instead.
		electorPeerAddr := c.AdvertisePeer
		if electionStatus.LeaderID != c.NodeID {
			reg, err := discovery.ReadNodeRegistration(ctx, storageClient, electionStatus.LeaderID)
			if err != nil {
				filteredLogger.Error("Failed to resolve elector peer address from service discovery",
					"leader_id", electionStatus.LeaderID,
					"error", err,
				)
				jitterWaitThenExit(filteredLogger)
			}
			electorPeerAddr = reg.PeerAdvertiseAddress
		}

		// Connect to the current Elector for peer RPCs
		if err := peerManager.ConnectElector(electionStatus.LeaderID, electorPeerAddr); err != nil {
			filteredLogger.Error("Failed to connect to elector", "error", err)
			jitterWaitThenExit(filteredLogger)
		}

		// Create and start heartbeat sender
		heartbeatSender := heartbeat.NewSender(
			filteredLogger.With("component", "heartbeat"),
			c.NodeID,
			state,
			peerManager,
			db,
			startTime,
			c.Elector.HeartbeatInterval.Duration,
			c.Replication.HeartbeatInterval.Duration,
		)
		go heartbeatSender.Run(ctx)

		// Create the replication follower.
		follower := replication.NewFollower(
			filteredLogger.With("component", "follower"),
			c.NodeID,
			state,
			peerManager,
			db,
			heartbeatSender,
			clientApiServer,
		)

		// Complete node loading and backfill before serving client traffic.
		bootstrapResult, err := bootstrap.Run(
			ctx,
			filteredLogger.With("component", "bootstrap"),
			c,
			state,
			db,
			storageClient,
			peerManager,
			electionRunner,
			follower,
		)
		if err != nil {
			filteredLogger.Error("bootstrap failed", "error", err)
			jitterWaitThenExit(filteredLogger)
		}
		snapshotWorker.InitializeWithSnapshot(bootstrapResult.LatestSnapshotInfo)
		snapshotWorker.Start()

		// Create Node gRPC server
		nodeSrv := node.NewServer(
			filteredLogger.With("component", "node-server"),
			c.NodeID,
			startTime,
			state,
			db,
			peerManager,
		)

		// Setup and run Peer gRPC server (Elector, Primary, and Node services)
		peerOpts := []grpc.ServerOption{
			grpc.Creds(credentials.NewTLS(tlsConfigPeerAPI)),
		}
		peerGRPCServer := grpc.NewServer(peerOpts...)
		proto.RegisterElectorServer(peerGRPCServer, electionRunner.ElectorServer())
		proto.RegisterPrimaryServer(peerGRPCServer, primarySrv)
		proto.RegisterNodeServer(peerGRPCServer, nodeSrv)
		peerListener, err := net.Listen("tcp", c.BindPeer)
		if err != nil {
			filteredLogger.Error("Unable to create peer gRPC server listener", "err", err)
			os.Exit(1)
		}
		filteredLogger.Info("starting peer (grpc) server...", "addr", c.BindPeer)
		peerShutdownCh := make(chan error, 1)
		go func() {
			peerShutdownCh <- peerGRPCServer.Serve(peerListener)
		}()

		// Run gRPC server with the etcd-compatible client API after the
		// node has completed bootstrap and become Healthy.
		grpcListener, err := net.Listen("tcp", c.BindClient)
		if err != nil {
			filteredLogger.Error("Unable to create gRPC server listener", "err", err)
			os.Exit(1)
		}
		filteredLogger.Info("starting client (grpc) server...", "addr", c.BindClient)
		shutdownCh := make(chan error, 1)
		go func() {
			shutdownCh <- grpcServer.Serve(grpcListener)
		}()

		// Wire role change hooks so the follower and primary services
		// are started and stopped on leadership transitions.
		peerManager.SetPrimaryChangeFunc(func(isPrimary bool) {
			if isPrimary {
				follower.Stop()
				primarySrv.StartServices(ctx)
			} else {
				primarySrv.StopServices()
				if err := follower.Start(ctx); err != nil {
					filteredLogger.Error("failed to start follower", "error", err)
				}
			}
		})

		// Bootstrap may already have established that this node is the current
		// Primary, so start Primary services immediately in that case.
		if state.ClusterState().Primary.NodeID == c.NodeID {
			primarySrv.StartServices(ctx)
		}

		// Block until SIGTERM/SIGINT or gRPC server error
		select {
		case sig := <-sigCh:
			filteredLogger.Info("received signal, starting shutdown", "signal", sig.String())
		case err := <-shutdownCh:
			filteredLogger.Error("client grpc server failed, starting shutdown", "error", err)
		case err := <-peerShutdownCh:
			filteredLogger.Error("peer grpc server failed, starting shutdown", "error", err)
		}
		signal.Stop(sigCh)
		cancel() // stop heartbeat sender and other background goroutines

		// Mark node as degraded health
		if err := state.SetHealth(nodestate.HealthDegraded); err != nil {
			filteredLogger.Error("failed to transition health to degraded", "error", err)
		}

		// Create a timeout for deregistration
		deregCtx, deregCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer deregCancel()

		// Deregister node with Elector
		if ec := peerManager.ElectorClient(); ec != nil {
			if _, err := ec.DeregisterNode(deregCtx, &proto.DeregisterNodeRequest{NodeId: c.NodeID}); err != nil {
				filteredLogger.Error("deregistration RPC failed", "error", err)
			}
		} else {
			filteredLogger.Warn("no elector connection available for deregistration")
		}

		// Delete node registration file in object storage
		if err := discovery.DeleteNodeRegistration(deregCtx, storageClient, c.NodeID); err != nil {
			filteredLogger.Error("failed to delete node registration file", "error", err)
		}

		// Close API servers and exit
		clientApiServer.Close()
		peerGRPCServer.GracefulStop()
		filteredLogger.Info("exiting")
	}

	return rootCmd
}

func jitterWaitThenExit(logger *slog.Logger) {
	waitFor := time.Duration(rand.Intn(10)) * time.Second
	logger.Info("waiting before exiting", "wait", waitFor)
	time.Sleep(waitFor)
	logger.Info("exiting...")
	os.Exit(1)
}
