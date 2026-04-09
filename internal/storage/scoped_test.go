// Copyright 2026 Nadrama Pty Ltd
// SPDX-License-Identifier: Apache-2.0

package storage

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
)

// recordingStorage records which keys were passed to each method
type recordingStorage struct {
	lastKey    string
	lastPrefix string
	listResult []ObjectInfo
}

func (r *recordingStorage) Put(ctx context.Context, key string, data io.Reader, size int64) error {
	r.lastKey = key
	return nil
}

func (r *recordingStorage) PutIfMatch(ctx context.Context, key string, data io.Reader, size int64, etag string) error {
	r.lastKey = key
	return nil
}

func (r *recordingStorage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	r.lastKey = key
	return io.NopCloser(strings.NewReader("")), nil
}

func (r *recordingStorage) Delete(ctx context.Context, key string) error {
	r.lastKey = key
	return nil
}

func (r *recordingStorage) List(ctx context.Context, prefix string) ([]ObjectInfo, error) {
	r.lastPrefix = prefix
	return r.listResult, nil
}

func TestScopedStorage_PrefixesKeys(t *testing.T) {
	inner := &recordingStorage{}
	scoped := newScopedStorage(inner, "myprefix")
	ctx := context.Background()

	scoped.Put(ctx, "chunks/file.netsy", nil, 0)
	if inner.lastKey != "myprefix/chunks/file.netsy" {
		t.Errorf("Put: got key %q, want %q", inner.lastKey, "myprefix/chunks/file.netsy")
	}

	scoped.PutIfMatch(ctx, "chunks/file.netsy", nil, 0, "")
	if inner.lastKey != "myprefix/chunks/file.netsy" {
		t.Errorf("PutIfMatch: got key %q, want %q", inner.lastKey, "myprefix/chunks/file.netsy")
	}

	scoped.Get(ctx, "snapshots/file.netsy")
	if inner.lastKey != "myprefix/snapshots/file.netsy" {
		t.Errorf("Get: got key %q, want %q", inner.lastKey, "myprefix/snapshots/file.netsy")
	}

	scoped.Delete(ctx, "nodes/node.json")
	if inner.lastKey != "myprefix/nodes/node.json" {
		t.Errorf("Delete: got key %q, want %q", inner.lastKey, "myprefix/nodes/node.json")
	}
}

func TestScopedStorage_ListPrefixesAndStrips(t *testing.T) {
	inner := &recordingStorage{
		listResult: []ObjectInfo{
			{Key: "myprefix/chunks/0001/0000000000000000001.netsy", Size: 100},
			{Key: "myprefix/chunks/0002/0000000000000000002.netsy", Size: 200},
		},
	}
	scoped := newScopedStorage(inner, "myprefix")
	ctx := context.Background()

	results, err := scoped.List(ctx, "chunks/")
	if err != nil {
		t.Fatalf("List: unexpected error: %v", err)
	}

	if inner.lastPrefix != "myprefix/chunks/" {
		t.Errorf("List: underlying got prefix %q, want %q", inner.lastPrefix, "myprefix/chunks/")
	}

	if len(results) != 2 {
		t.Fatalf("List: got %d results, want 2", len(results))
	}

	// Keys should have prefix stripped
	if results[0].Key != "chunks/0001/0000000000000000001.netsy" {
		t.Errorf("List: results[0].Key = %q, want prefix stripped", results[0].Key)
	}
	if results[1].Key != "chunks/0002/0000000000000000002.netsy" {
		t.Errorf("List: results[1].Key = %q, want prefix stripped", results[1].Key)
	}
}

func TestScopedStorage_TrailingSlash(t *testing.T) {
	inner := &recordingStorage{}

	// Without trailing slash
	scoped1 := newScopedStorage(inner, "prefix")
	scoped1.Put(context.Background(), "key", nil, 0)
	if inner.lastKey != "prefix/key" {
		t.Errorf("without slash: got %q, want %q", inner.lastKey, "prefix/key")
	}

	// With trailing slash
	scoped2 := newScopedStorage(inner, "prefix/")
	scoped2.Put(context.Background(), "key", nil, 0)
	if inner.lastKey != "prefix/key" {
		t.Errorf("with slash: got %q, want %q", inner.lastKey, "prefix/key")
	}
}

func TestScopedStorage_ErrorPropagation(t *testing.T) {
	inner := &errorStorage{}
	scoped := newScopedStorage(inner, "prefix")

	_, err := scoped.List(context.Background(), "chunks/")
	if err == nil {
		t.Fatal("List: expected error, got nil")
	}
}

type errorStorage struct{}

func (e *errorStorage) Put(ctx context.Context, key string, data io.Reader, size int64) error {
	return fmt.Errorf("put error")
}
func (e *errorStorage) PutIfMatch(ctx context.Context, key string, data io.Reader, size int64, etag string) error {
	return fmt.Errorf("put error")
}
func (e *errorStorage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	return nil, fmt.Errorf("get error")
}
func (e *errorStorage) Delete(ctx context.Context, key string) error {
	return fmt.Errorf("delete error")
}
func (e *errorStorage) List(ctx context.Context, prefix string) ([]ObjectInfo, error) {
	return nil, fmt.Errorf("list error")
}
