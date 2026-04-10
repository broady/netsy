// Copyright 2026 Nadrama Pty Ltd
// SPDX-License-Identifier: Apache-2.0

package nodestate

import (
	"github.com/nadrama-com/netsy/internal/proto"
)

// HealthFromProto converts a proto HealthState enum to the internal
// HealthState value.
func HealthFromProto(h proto.HealthState) HealthState {
	switch h {
	case proto.HealthState_HEALTH_HEALTHY:
		return HealthHealthy
	case proto.HealthState_HEALTH_DEGRADED:
		return HealthDegraded
	default:
		return HealthLoading
	}
}

// HealthToProto converts an internal HealthState to the proto enum.
func HealthToProto(h HealthState) proto.HealthState {
	switch h {
	case HealthHealthy:
		return proto.HealthState_HEALTH_HEALTHY
	case HealthDegraded:
		return proto.HealthState_HEALTH_DEGRADED
	case HealthLoading:
		return proto.HealthState_HEALTH_LOADING
	default:
		return proto.HealthState_HEALTH_UNKNOWN
	}
}

// ElectorFromProto converts a proto ElectorState enum to the internal
// ElectorState value.
func ElectorFromProto(e proto.ElectorState) ElectorState {
	switch e {
	case proto.ElectorState_ELECTOR_LEADER:
		return ElectorLeader
	default:
		return ElectorFollower
	}
}

// ElectorToProto converts an internal ElectorState to the proto enum.
func ElectorToProto(e ElectorState) proto.ElectorState {
	switch e {
	case ElectorLeader:
		return proto.ElectorState_ELECTOR_LEADER
	case ElectorFollower:
		return proto.ElectorState_ELECTOR_FOLLOWER
	default:
		return proto.ElectorState_ELECTOR_UNKNOWN
	}
}

// PrimaryFromProto converts a proto PrimaryState enum to the internal
// PrimaryState value.
func PrimaryFromProto(p proto.PrimaryState) PrimaryState {
	switch p {
	case proto.PrimaryState_PRIMARY_STARTING:
		return PrimaryStarting
	case proto.PrimaryState_PRIMARY_ACTIVE:
		return PrimaryActive
	case proto.PrimaryState_PRIMARY_DRAINING:
		return PrimaryDraining
	default:
		return PrimaryReplica
	}
}

// PrimaryToProto converts an internal PrimaryState to the proto enum.
func PrimaryToProto(p PrimaryState) proto.PrimaryState {
	switch p {
	case PrimaryStarting:
		return proto.PrimaryState_PRIMARY_STARTING
	case PrimaryActive:
		return proto.PrimaryState_PRIMARY_ACTIVE
	case PrimaryDraining:
		return proto.PrimaryState_PRIMARY_DRAINING
	case PrimaryReplica:
		return proto.PrimaryState_PRIMARY_REPLICA
	default:
		return proto.PrimaryState_PRIMARY_UNKNOWN
	}
}
