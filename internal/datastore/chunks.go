// Copyright 2026 Nadrama Pty Ltd
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"sort"

	"github.com/nadrama-com/netsy/internal/datafile"
	pb "github.com/nadrama-com/netsy/internal/proto"
	"github.com/nadrama-com/netsy/internal/storage"
)

// MarshalChunk serialises a single record into the chunk datafile format,
// returning the object-storage key and the raw bytes.
func MarshalChunk(record *pb.Record, nodeID string) (key string, data []byte, err error) {
	var buf bytes.Buffer
	w, err := datafile.NewWriter(bufio.NewWriter(&buf), pb.FileKind_KIND_CHUNK, 1, nodeID)
	if err != nil {
		return "", nil, fmt.Errorf("create datafile writer: %w", err)
	}
	if err := w.Write(record); err != nil {
		return "", nil, fmt.Errorf("write record: %w", err)
	}
	if err := w.Close(); err != nil {
		return "", nil, fmt.Errorf("close datafile writer: %w", err)
	}
	return ChunkKey(record.Revision), buf.Bytes(), nil
}

// ChunkKey generates the object storage key for a chunk file
func ChunkKey(revision int64) string {
	// Format: chunks/{partition}/{zero-padded-revision}.netsy
	// Partition is modulo 10000 to avoid hot paths
	// Revision is zero-padded to 19 characters (max int64)
	partition := revision % 10000
	return fmt.Sprintf("chunks/%04d/%019d.netsy", partition, revision)
}

// ListChunks returns chunk files with revision > fromRevision, sorted oldest first
func ListChunks(ctx context.Context, store storage.ObjectStorage, fromRevision int64) ([]FileInfo, error) {
	objects, err := store.List(ctx, "chunks/")
	if err != nil {
		return nil, fmt.Errorf("failed to list chunk objects: %w", err)
	}

	var chunks []FileInfo
	for _, obj := range objects {
		rev, ok := parseRevisionFromKey(obj.Key)
		if !ok {
			continue
		}
		if rev > fromRevision {
			chunks = append(chunks, FileInfo{Key: obj.Key, Size: obj.Size, Revision: rev})
		}
	}

	sort.Slice(chunks, func(i, j int) bool {
		return chunks[i].Revision < chunks[j].Revision
	})

	return chunks, nil
}

// ListChunksForCleanup returns chunk files with revision <= upToRevision, sorted oldest first
func ListChunksForCleanup(ctx context.Context, store storage.ObjectStorage, upToRevision int64) ([]FileInfo, error) {
	objects, err := store.List(ctx, "chunks/")
	if err != nil {
		return nil, fmt.Errorf("failed to list chunk objects for cleanup: %w", err)
	}

	var chunks []FileInfo
	for _, obj := range objects {
		rev, ok := parseRevisionFromKey(obj.Key)
		if !ok {
			continue
		}
		if rev <= upToRevision {
			chunks = append(chunks, FileInfo{Key: obj.Key, Size: obj.Size, Revision: rev})
		}
	}

	sort.Slice(chunks, func(i, j int) bool {
		return chunks[i].Revision < chunks[j].Revision
	})

	return chunks, nil
}
