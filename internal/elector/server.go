// Netsy <https://netsy.dev>
// Copyright 2026 Nadrama Pty Ltd
// SPDX-License-Identifier: Apache-2.0

package elector

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/nadrama-com/netsy/internal/discovery"
	"github.com/nadrama-com/netsy/internal/nodestate"
	"github.com/nadrama-com/netsy/internal/peerclient"
	"github.com/nadrama-com/netsy/internal/proto"
	"github.com/nadrama-com/netsy/internal/storage"
)

// RevisionSource provides the latest revision for election tie-breaking.
type RevisionSource interface {
	LatestRevision() (int64, error)
}

// Server implements the proto.ElectorServer gRPC interface. It is only
// active when the local node is the Elector (leader).
type Server struct {
	proto.UnimplementedElectorServer

	logger            *slog.Logger
	clusterID         string
	store             storage.ObjectStorage
	state             *nodestate.State
	nodeMap           *NodeMap
	deregTimeout      time.Duration
	heartbeatInterval time.Duration
	degradationCount  int

	// Fields for primary election.
	localNodeID         string
	localStartTime      int64
	localDB             RevisionSource
	quorum              int
	primaryPriorTimeout time.Duration
	peers               *peerclient.Manager

	// previousPrimary holds the identity of the last known Primary so
	// that checkPreviousPrimary can contact it for a drain check even
	// after the Primary has been cleared from ClusterState.
	previousPrimary nodestate.NodeInfo
}

// NewServer creates a new Elector gRPC server.
func NewServer(
	logger *slog.Logger,
	clusterID string,
	store storage.ObjectStorage,
	state *nodestate.State,
	deregTimeout time.Duration,
	heartbeatInterval time.Duration,
	degradationCount int,
	localNodeID string,
	localStartTime int64,
	localDB RevisionSource,
	quorum int,
	primaryPriorTimeout time.Duration,
	peers *peerclient.Manager,
) *Server {
	return &Server{
		logger:              logger,
		clusterID:           clusterID,
		store:               store,
		state:               state,
		nodeMap:              NewNodeMap(logger.With("component", "node-map")),
		deregTimeout:        deregTimeout,
		heartbeatInterval:   heartbeatInterval,
		degradationCount:    degradationCount,
		localNodeID:         localNodeID,
		localStartTime:      localStartTime,
		localDB:             localDB,
		quorum:              quorum,
		primaryPriorTimeout: primaryPriorTimeout,
		peers:               peers,
	}
}

// RegisterNode registers a node with the Elector, allocating or reusing a
// member_id. It returns the assigned member_id and the current cluster state.
func (s *Server) RegisterNode(ctx context.Context, req *proto.RegisterNodeRequest) (resp *proto.RegisterNodeResponse, err error) {
	if err := s.requireLeader(); err != nil {
		return nil, err
	}
	if req.GetNodeId() == "" || req.GetClientAdvertiseAddress() == "" || req.GetPeerAdvertiseAddress() == "" {
		return nil, status.Error(codes.InvalidArgument, "node_id, client_advertise_address, and peer_advertise_address are required")
	}
	if !s.nodeMap.Ready() {
		return nil, status.Error(codes.Unavailable, "elector is still bootstrapping")
	}

	s.logger.Info("registering node",
		"node_id", req.GetNodeId(),
		"client_addr", req.GetClientAdvertiseAddress(),
		"peer_addr", req.GetPeerAdvertiseAddress(),
	)

	memberID, err := s.allocateOrReuseMemberID(ctx, req.GetNodeId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to allocate member_id: %v", err)
	}

	s.nodeMap.Add(NodeEntry{
		NodeID:                 req.GetNodeId(),
		MemberID:               memberID,
		ClientAdvertiseAddress: req.GetClientAdvertiseAddress(),
		PeerAdvertiseAddress:   req.GetPeerAdvertiseAddress(),
		LastHeartbeat:          time.Now(),
		HealthState:            nodestate.HealthLoading,
	})

	cs := s.state.ClusterState()
	cs.NodeCount = s.nodeMap.Count()
	protoCS := nodestate.ClusterStateToProto(cs)

	return &proto.RegisterNodeResponse{
		MemberId:     memberID,
		ClusterState: protoCS,
	}, nil
}

// DeregisterNode removes a node from the Elector's node map and deletes
// its registration file. The durable member_id mapping in members.json is
// preserved for future re-registration.
func (s *Server) DeregisterNode(ctx context.Context, req *proto.DeregisterNodeRequest) (_ *emptypb.Empty, err error) {
	if err := s.requireLeader(); err != nil {
		return nil, err
	}
	if req.GetNodeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "node_id is required")
	}

	nodeID := req.GetNodeId()
	s.logger.Info("deregistering node", "node_id", nodeID)

	s.nodeMap.MarkDeregistered(nodeID)
	s.nodeMap.Remove(nodeID)

	if cs := s.state.ClusterState(); cs.Primary.NodeID == nodeID {
		s.clearPrimary(ctx, "primary deregistered via RPC")
	}

	return &emptypb.Empty{}, nil
}

// GetClusterState returns the current cluster state as known by the Elector.
func (s *Server) GetClusterState(_ context.Context, _ *emptypb.Empty) (resp *proto.ClusterState, err error) {
	if err := s.requireLeader(); err != nil {
		return nil, err
	}
	if !s.nodeMap.Ready() {
		return nil, status.Error(codes.Unavailable, "elector is still bootstrapping")
	}

	cs := s.state.ClusterState()
	cs.NodeCount = s.nodeMap.Count()
	return nodestate.ClusterStateToProto(cs), nil
}

// SendHeartbeat receives a NodeState heartbeat from a Node, updating the
// node map with the latest heartbeat timestamp and reported state.
func (s *Server) SendHeartbeat(_ context.Context, req *proto.NodeState) (_ *emptypb.Empty, err error) {
	if err := s.requireLeader(); err != nil {
		return nil, err
	}
	if req.GetNodeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "node_id is required")
	}

	health := nodestate.HealthFromProto(req.GetHealthState())
	primary := nodestate.PrimaryFromProto(req.GetPrimaryState())

	if !s.nodeMap.UpdateHeartbeat(req.GetNodeId(), time.Now(), health, primary, req.GetLatestRevision(), req.GetStartTime()) {
		return nil, status.Errorf(codes.NotFound, "node %s is not registered", req.GetNodeId())
	}

	return &emptypb.Empty{}, nil
}

// requireLeader returns a gRPC error if this node is not the Elector leader.
func (s *Server) requireLeader() error {
	if s.state.Elector() != nodestate.ElectorLeader {
		return status.Error(codes.FailedPrecondition, "this node is not the elector")
	}
	return nil
}

// GetMemberList returns all registered nodes from the Elector's in-memory
// node map. Only callable when this node is the Elector leader and the
// node map bootstrap has completed.
func (s *Server) GetMemberList(_ context.Context, _ *proto.GetMemberListRequest) (*proto.GetMemberListResponse, error) {
	if err := s.requireLeader(); err != nil {
		return nil, err
	}
	if !s.nodeMap.Ready() {
		return nil, status.Error(codes.Unavailable, "elector is still bootstrapping")
	}

	entries := s.nodeMap.All()
	members := make([]*proto.MemberEntry, 0, len(entries))
	for _, e := range entries {
		members = append(members, &proto.MemberEntry{
			NodeId:                 e.NodeID,
			MemberId:               e.MemberID,
			ClientAdvertiseAddress: e.ClientAdvertiseAddress,
			PeerAdvertiseAddress:   e.PeerAdvertiseAddress,
		})
	}

	return &proto.GetMemberListResponse{Members: members}, nil
}

// allocateOrReuseMemberID reads members.json, reuses an existing member_id
// for the node if present, or allocates a new one. The updated members.json
// is written back with a conditional write and retried on precondition failure.
func (s *Server) allocateOrReuseMemberID(ctx context.Context, nodeID string) (memberID uint64, err error) {
	const maxRetries = 5

	for attempt := range maxRetries {
		mf, err := discovery.ReadMembersFile(ctx, s.store)
		if err != nil {
			return 0, fmt.Errorf("read members file: %w", err)
		}

		if id, ok := discovery.FindMemberID(mf, nodeID); ok {
			return id, nil
		}

		newID := discovery.AllocateMemberID(mf)
		mf.Members[nodeID] = newID

		if err := discovery.WriteMembersFile(ctx, s.store, mf); err != nil {
			if errors.Is(err, storage.ErrPrecondition) {
				s.logger.Info("members.json write conflict, retrying",
					"attempt", attempt+1,
					"node_id", nodeID,
				)
				continue
			}
			return 0, fmt.Errorf("write members file: %w", err)
		}
		return newID, nil
	}
	return 0, fmt.Errorf("failed to allocate member_id after %d retries", maxRetries)
}


