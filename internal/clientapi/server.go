// Copyright 2026 Nadrama Pty Ltd
// SPDX-License-Identifier: Apache-2.0

package clientapi

import (
	"log/slog"

	"github.com/nadrama-com/netsy/internal/config"
	"github.com/nadrama-com/netsy/internal/localdb"
	"github.com/nadrama-com/netsy/internal/primary"

	pb "go.etcd.io/etcd/api/v3/etcdserverpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

// ClientAPIServer implements a gRPC server compatible with the Kubernetes etcd API subset
// @see https://github.com/etcd-io/etcd/blob/main/api/etcdserverpb/rpc.proto#L37
// @see https://github.com/etcd-io/etcd/blob/main/api/etcdserverpb/rpc.pb.go
// etcd has the following gRPC "services":
// * KV
// * Watch
// * Lease
// * Cluster
// * Maintenance
// * Auth
// we include the 'Unimplemented' services by default and override them where required
type ClientAPIServer struct {
	logger     *slog.Logger
	config     *config.Config
	db         localdb.Database
	grpcServer *grpc.Server
	// note: in future we will replace this with a peer server gRPC client
	peerServer *primary.Server
	// note: sending messages not currently required
	//wsSendCh     chan []byte
	pb.UnimplementedKVServer
	pb.UnimplementedWatchServer
	pb.UnimplementedLeaseServer
	pb.UnimplementedClusterServer
	pb.UnimplementedMaintenanceServer
	pb.UnimplementedAuthServer
}

// NewServer registers the etcd-compatible Client API services on the provided gRPC server.
func NewServer(logger *slog.Logger, conf *config.Config, db localdb.Database, grpcServer *grpc.Server, peerServer *primary.Server) *ClientAPIServer {
	clientServer := &ClientAPIServer{
		logger:     logger,
		config:     conf,
		grpcServer: grpcServer,
		db:         db,
		// TODO: in future we will replace this with a peer server gRPC client
		// when the Netsy server is not the leader
		peerServer: peerServer,
	}

	pb.RegisterKVServer(grpcServer, clientServer)
	pb.RegisterWatchServer(grpcServer, clientServer)
	pb.RegisterLeaseServer(grpcServer, clientServer)
	pb.RegisterClusterServer(grpcServer, clientServer)
	pb.RegisterMaintenanceServer(grpcServer, clientServer)
	pb.RegisterAuthServer(grpcServer, clientServer)
	hsrv := health.NewServer()
	hsrv.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(grpcServer, hsrv)
	reflection.Register(grpcServer)

	return clientServer
}

func (clientServer *ClientAPIServer) Close() {
	clientServer.grpcServer.GracefulStop()
	clientServer.db.Close()
}
