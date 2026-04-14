// Copyright 2026 Nadrama Pty Ltd
// SPDX-License-Identifier: Apache-2.0

package primary

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/nadrama-com/netsy/internal/datastore"
	pb "github.com/nadrama-com/netsy/internal/proto"
	"github.com/nadrama-com/netsy/internal/storage"
)

// errChunkAlreadyExists reports that a strict create-only chunk write found an
// existing object at the target revision key.
var errChunkAlreadyExists = errors.New("chunk already exists")

// writeRecord writes a single record to object storage as a chunk file.
func (ps *Server) writeRecord(ctx context.Context, record *pb.Record) error {
	key, data, err := datastore.MarshalChunk(record, ps.config.NodeID)
	if err != nil {
		return err
	}

	err = ps.storageClient.PutIfMatch(ctx, key, data, "")
	if err != nil {
		ps.logger.Debug("first upload attempt failed, retrying once", "error", err, "key", key)
		err = ps.storageClient.PutIfMatch(ctx, key, data, "")
		if err != nil {
			if errors.Is(err, storage.ErrPrecondition) {
				return fmt.Errorf("%w: %s: %w", errChunkAlreadyExists, key, err)
			}
			return fmt.Errorf("object storage upload failed after retry: %w", err)
		}
		ps.logger.Info("object storage upload succeeded on retry", "key", key)
	}

	ps.logger.Debug("record written to object storage", "revision", record.Revision, "key", key)
	return nil
}

// writeRecordIfMissing ensures a record's chunk file exists in object storage,
// tolerating an already-present identical file from an earlier retry.
func (ps *Server) writeRecordIfMissing(ctx context.Context, record *pb.Record) error {
	err := ps.writeRecord(ctx, record)
	if err == nil {
		return nil
	}
	if !errors.Is(err, errChunkAlreadyExists) {
		return err
	}

	// Chunk already exists. Verify it matches what we would have written.
	key, data, marshalErr := datastore.MarshalChunk(record, ps.config.NodeID)
	if marshalErr != nil {
		return marshalErr
	}

	existing, _, getErr := ps.storageClient.Get(ctx, key)
	if getErr != nil {
		return fmt.Errorf("read existing chunk %s after already-exists: %w", key, getErr)
	}
	if !bytes.Equal(existing, data) {
		return fmt.Errorf("chunk %s already exists with different contents", key)
	}

	ps.logger.Debug("record already durable in object storage during primary preflight",
		"revision", record.Revision,
		"key", key,
	)
	return nil
}
