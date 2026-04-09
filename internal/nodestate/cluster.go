// Copyright 2026 Nadrama Pty Ltd
// SPDX-License-Identifier: Apache-2.0

package nodestate

// NodeInfo identifies a Node and how to reach it.
type NodeInfo struct {
	NodeID            string
	MemberID          uint64
	PeerAdvertiseAddr string
}

// ClusterState holds the current Elector and Primary as seen by this Node.
type ClusterState struct {
	Elector NodeInfo
	Primary NodeInfo
}

// ClusterState returns a copy of the current ClusterState.
func (s *State) ClusterState() ClusterState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cluster
}

// SetClusterState replaces the current ClusterState.
func (s *State) SetClusterState(cs ClusterState) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cluster = cs
	s.logger.Info("cluster_state_updated",
		"elector_node_id", cs.Elector.NodeID,
		"elector_peer_addr", cs.Elector.PeerAdvertiseAddr,
		"primary_node_id", cs.Primary.NodeID,
		"primary_peer_addr", cs.Primary.PeerAdvertiseAddr,
	)
}

// SetClusterElector updates only the Elector in the ClusterState.
func (s *State) SetClusterElector(elector NodeInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cluster.Elector = elector
	s.logger.Info("cluster_state_updated",
		"elector_node_id", elector.NodeID,
		"elector_peer_addr", elector.PeerAdvertiseAddr,
	)
}
