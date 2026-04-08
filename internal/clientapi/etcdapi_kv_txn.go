// Copyright 2025 Nadrama Pty Ltd
// SPDX-License-Identifier: Apache-2.0

package clientapi

import (
	"context"
	"errors"

	"github.com/nadrama-com/netsy/internal/localdb"
	"github.com/nadrama-com/netsy/internal/proto"
	pb "go.etcd.io/etcd/api/v3/etcdserverpb"
)

func (cs *ClientAPIServer) Txn(ctx context.Context, r *pb.TxnRequest) (resp *pb.TxnResponse, err error) {
	// Process transaction on leader
	inserted, resp, err := cs.peerServer.LeaderTxn(ctx, r)
	// If any type of error occurs, logs and then always return well-formed error response
	if err != nil {
		if errors.Is(err, localdb.ErrCompareRevisionFailed) ||
			errors.Is(err, localdb.ErrCreateKeyExists) ||
			errors.Is(err, localdb.ErrDeleteKeyNotFound) {
			if len(r.Failure) > 0 {
				cs.logger.Debug("txn error", "txnerror", err.Error())
			} else {
				cs.logger.Info("txn error", "txnerror", err.Error())
			}
		} else {
			cs.logger.Error("txn error", "txnerror", err.Error())
		}
		// Best-effort latest revision retrieval
		// If this fails we still want to return a well formed error
		latestRevision, _ := cs.db.LatestRevision()
		resp = &pb.TxnResponse{
			Header: &pb.ResponseHeader{
				Revision: latestRevision,
			},
		}
	} else if inserted != nil && inserted.Created {
		cs.logger.Debug("txn created", "key", string(inserted.Key), "rev", inserted.Revision)
	} else if inserted != nil && inserted.Deleted {
		cs.logger.Debug("txn deleted", "key", string(inserted.Key), "rev", inserted.Revision)
	} else if inserted != nil {
		cs.logger.Debug("txn updated", "key", string(inserted.Key), "rev", inserted.Revision)
	}
	// Replicate to watchers
	var prevRecord *proto.Record
	if inserted != nil && !inserted.Created && inserted.PrevRevision > 0 {
		prevRecord, err = cs.db.FindRecordByRev(inserted.PrevRevision)
		if err != nil {
			cs.logger.Debug("find prev", "key", string(inserted.Key), "rev", inserted.Revision, "prev", inserted.PrevRevision, "err", err.Error())
		}
	}
	if inserted != nil {
		cs.Distribute(inserted, prevRecord)
	}
	return resp, nil
}
