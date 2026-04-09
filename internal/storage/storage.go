// Copyright 2026 Nadrama Pty Ltd
// SPDX-License-Identifier: Apache-2.0

package storage

import (
	"context"
	"io"

	"github.com/nadrama-com/s3lect"
)

// Common storage errors — use s3lect's errors for compatibility.
var (
	ErrNotFound     = s3lect.ErrStorageNotFound
	ErrPrecondition = s3lect.ErrStoragePrecondition
)

// ObjectInfo represents metadata about an object in storage.
type ObjectInfo struct {
	Key  string
	Size int64
}

// ObjectStorage defines the interface for object storage operations.
// Buffered methods (Get, Put, PutIfMatch) operate on whole objects in memory
// and are intended for small blobs/files. Streaming methods
// (GetStream, PutStream) are intended for large data files.
type ObjectStorage interface {
	// Get retrieves an object and returns its contents and ETag.
	// Returns ErrNotFound when the key does not exist.
	Get(ctx context.Context, key string) ([]byte, string, error)

	// Put stores an object in storage.
	Put(ctx context.Context, key string, data []byte) error

	// PutIfMatch stores an object only if the ETag matches.
	// An empty etag means the object must not exist (create-only).
	// Returns ErrPrecondition when the precondition is not met.
	PutIfMatch(ctx context.Context, key string, data []byte, etag string) error

	// GetStream retrieves an object as a stream.
	// Returns ErrNotFound when the key does not exist.
	GetStream(ctx context.Context, key string) (io.ReadCloser, error)

	// PutStream stores an object from a stream.
	PutStream(ctx context.Context, key string, r io.Reader, size int64) error

	// Delete removes the object at the given key.
	Delete(ctx context.Context, key string) error

	// List returns all object keys matching the given prefix.
	List(ctx context.Context, prefix string) ([]ObjectInfo, error)
}
