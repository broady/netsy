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

	"github.com/nadrama-com/netsy/internal"
	"github.com/nadrama-com/netsy/internal/buildvars"
	"github.com/nadrama-com/netsy/internal/clientapi"
	"github.com/nadrama-com/netsy/internal/config"
	"github.com/nadrama-com/netsy/internal/datastore"
	"github.com/nadrama-com/netsy/internal/discovery"
	"github.com/nadrama-com/netsy/internal/elector"
	"github.com/nadrama-com/netsy/internal/healthserver"
	"github.com/nadrama-com/netsy/internal/localdb"
	"github.com/nadrama-com/netsy/internal/mtls"
	"github.com/nadrama-com/netsy/internal/nodestate"
	"github.com/nadrama-com/netsy/internal/proto"
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

		// Start elector - s3lect election, health server, and elector service
		electionRunner, err := elector.New(filteredLogger, c, state, storageClient, tlsFiles.ServerCert)
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

		// Establish gRPC connection to the current Elector for peer RPCs
		electorConn, err := grpc.NewClient(
			electionStatus.LeaderAddr,
			grpc.WithTransportCredentials(credentials.NewTLS(tlsConfigPeerClient)),
		)
		if err != nil {
			filteredLogger.Error("Failed to create elector client connection", "error", err)
			jitterWaitThenExit(filteredLogger)
		}
		defer electorConn.Close()
		electorClient := proto.NewElectorClient(electorConn)

		// Instantiate database
		db := localdb.New(fmt.Sprintf("%s/db.sqlite3", c.DataDir))
		err = db.Connect()
		if err != nil {
			filteredLogger.Error("db.Connect error", "error", err)
			jitterWaitThenExit(filteredLogger)
		}

		// Backfill and verify database
		latestRevision, err := db.LatestRevision()
		if err != nil {
			filteredLogger.Error("db.LatestRevision error", "error", err)
			jitterWaitThenExit(filteredLogger)
		}

		// Get latest snapshot info
		var snapshotWorker *snapshot.Worker
		var latestSnapshotInfo *datastore.LatestSnapshotInfo
		latestSnapshotInfo, err = datastore.GetLatestSnapshot(context.Background(), storageClient)
		if err != nil {
			filteredLogger.Error("Failed to get latest snapshot info", "error", err)
			os.Exit(1)
		}

		snapshotWorker = snapshot.NewWorker(filteredLogger, c, db, storageClient)
		snapshotWorker.InitializeWithSnapshot(latestSnapshotInfo)

		// Ensure snapshot worker is stopped on shutdown
		defer func() {
			filteredLogger.Info("shutting down snapshot worker")
			snapshotWorker.Stop()
		}()

		err = internal.Backfill(filteredLogger, db, c, latestRevision, latestSnapshotInfo, storageClient)
		if err != nil {
			filteredLogger.Error("clientServer.Backfill error", "error", err)
			jitterWaitThenExit(filteredLogger)
		}
		err = db.VerifyIntegrity()
		if err != nil {
			filteredLogger.Error("clientServer.db.VerifyIntegrity error", "error", err)
			jitterWaitThenExit(filteredLogger)
		}

		// Start snapshot worker after backfill is complete
		snapshotWorker.Start()

		// Setup and run gRPC server with (etcd-compatible) client API
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
		clientApiServer, err := clientapi.NewServer(filteredLogger, c, db, grpcServer, snapshotWorker, storageClient)
		if err != nil {
			filteredLogger.Error("Unable to create server client", "err", err)
			os.Exit(1)
		}
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

		// Block until SIGTERM/SIGINT or gRPC server error
		select {
		case sig := <-sigCh:
			filteredLogger.Info("received signal, starting shutdown", "signal", sig.String())
		case err := <-shutdownCh:
			filteredLogger.Error("grpc server failed, starting shutdown", "error", err)
		}
		signal.Stop(sigCh)

		// Mark node as degraded health
		if err := state.SetHealth(nodestate.HealthDegraded); err != nil {
			filteredLogger.Error("failed to transition health to degraded", "error", err)
		}

		// Create a timeout for deregistration
		deregCtx, deregCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer deregCancel()

		// Deregister node with Elector
		if _, err := electorClient.DeregisterNode(deregCtx, &proto.DeregisterNodeRequest{NodeId: c.NodeID}); err != nil {
			filteredLogger.Error("deregistration RPC failed", "error", err)
		}

		// Delete node registration file in object storage
		if err := discovery.DeleteNodeRegistration(deregCtx, storageClient, c.NodeID); err != nil {
			filteredLogger.Error("failed to delete node registration file", "error", err)
		}

		// Close API servers and exit
		clientApiServer.Close()
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
