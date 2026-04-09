// Copyright 2026 Nadrama Pty Ltd
// SPDX-License-Identifier: Apache-2.0

package primary

import (
	"bufio"
	"bytes"
	"context"
	"fmt"

	"github.com/nadrama-com/netsy/internal/datafile"
	"github.com/nadrama-com/netsy/internal/datastore"
	pb "github.com/nadrama-com/netsy/internal/proto"
)

// writeRecord writes a single record to object storage as a chunk file
func (ps *Server) writeRecord(ctx context.Context, record *pb.Record) error {
	buffer := &bytes.Buffer{}
	bufWriter := bufio.NewWriter(buffer)

	// Create datafile writer for a single record chunk
	// Use the instance ID from config as the leader ID
	leaderID := ps.config.NodeID
	writer, err := datafile.NewWriter(bufWriter, pb.FileKind_KIND_CHUNK, 1, leaderID)
	if err != nil {
		return fmt.Errorf("failed to create datafile writer: %w", err)
	}

	err = writer.Write(record)
	if err != nil {
		return fmt.Errorf("failed to write record: %w", err)
	}

	err = writer.Close()
	if err != nil {
		return fmt.Errorf("failed to close datafile writer: %w", err)
	}

	// Generate object storage key for the chunk file
	key := datastore.ChunkKey(record.Revision)
	data := buffer.Bytes()

	// Upload with retry-once logic
	err = ps.storageClient.PutIfMatch(ctx, key, bytes.NewReader(data), int64(len(data)), "")
	if err != nil {
		ps.logger.Debug("first upload attempt failed, retrying once", "error", err, "key", key)
		// Retry once on failure
		err = ps.storageClient.PutIfMatch(ctx, key, bytes.NewReader(data), int64(len(data)), "")
		if err != nil {
			return fmt.Errorf("object storage upload failed after retry: %w", err)
		}
		ps.logger.Info("object storage upload succeeded on retry", "key", key)
	}

	ps.logger.Debug("record written to object storage", "revision", record.Revision, "key", key)
	return nil
}
