// Netsy <https://netsy.dev>
// Copyright 2026 Nadrama Pty Ltd
// SPDX-License-Identifier: Apache-2.0

package clientapi

import (
	"context"

	"github.com/nadrama-com/netsy/internal/commonapi"
	pb "go.etcd.io/etcd/api/v3/etcdserverpb"
)

// Range serves a read request limited to the committed revision. Records
// above the committed revision are tentative and must not be visible to
// clients.
func (cs *ClientAPIServer) Range(ctx context.Context, r *pb.RangeRequest) (*pb.RangeResponse, error) {
	committed := cs.state.Committed()

	// Limit the request revision to the committed revision so tentative
	// records above it are never served.
	if r.Revision == 0 || r.Revision > committed {
		r.Revision = committed
	}

	return commonapi.Range(cs.db, ctx, r)
}
