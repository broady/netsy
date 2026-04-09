// Copyright 2026 Nadrama Pty Ltd
// SPDX-License-Identifier: Apache-2.0

package storage

import (
	"context"
	"io"
)

// ObjectInfo represents metadata about an object in storage
type ObjectInfo struct {
	Key  string
	Size int64
}

// ObjectStorage defines the interface for object storage operations
type ObjectStorage interface {
	// Put writes data to the given key
	Put(ctx context.Context, key string, data io.Reader, size int64) error

	// PutIfMatch writes data conditionally. If etag is empty, the object must
	// not already exist (create-only). If etag is non-empty, the existing
	// object's etag must match (compare-and-swap).
	PutIfMatch(ctx context.Context, key string, data io.Reader, size int64, etag string) error

	// Get returns a reader for the object at the given key
	Get(ctx context.Context, key string) (io.ReadCloser, error)

	// Delete removes the object at the given key
	Delete(ctx context.Context, key string) error

	// List returns all object keys matching the given prefix
	List(ctx context.Context, prefix string) ([]ObjectInfo, error)
}
