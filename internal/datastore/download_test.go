// Netsy <https://netsy.dev>
// Copyright 2026 Nadrama Pty Ltd
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"log/slog"

	"github.com/netsy-dev/netsy/internal/datafile"
	"github.com/netsy-dev/netsy/internal/localdb"
	pb "github.com/netsy-dev/netsy/internal/proto"
	"github.com/netsy-dev/netsy/internal/storage"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TestDownloadAndImportFile imports a chunk file from object storage and
// replays its records into SQLite.
func TestDownloadAndImportFile(t *testing.T) {
	db := openDatastoreTestDB(t)
	store := storage.NewMemoryStore()
	record := &pb.Record{
		Revision:       1,
		Key:            []byte("key"),
		Value:          []byte("value"),
		Created:        true,
		CreateRevision: 1,
		Version:        1,
		CreatedAt:      timestamppb.New(time.Unix(1, 0).UTC()),
		LeaderId:       "leader-1",
	}

	payload := encodeDatastoreTestFile(t, pb.FileKind_KIND_CHUNK, record)
	if err := store.Put(context.Background(), ChunkKey(record.GetRevision()), payload); err != nil {
		t.Fatalf("store.Put() error = %v", err)
	}

	if err := DownloadAndImportFile(
		context.Background(),
		slog.Default(),
		db,
		store,
		t.TempDir(),
		ChunkKey(record.GetRevision()),
		int64(len(payload)),
		pb.FileKind_KIND_CHUNK,
		nil,
	); err != nil {
		t.Fatalf("DownloadAndImportFile() error = %v", err)
	}

	got, err := db.FindRecordByRev(record.GetRevision())
	if err != nil {
		t.Fatalf("FindRecordByRev() error = %v", err)
	}
	if string(got.GetKey()) != string(record.GetKey()) {
		t.Fatalf("FindRecordByRev().Key = %q, want %q", got.GetKey(), record.GetKey())
	}
	if string(got.GetValue()) != string(record.GetValue()) {
		t.Fatalf("FindRecordByRev().Value = %q, want %q", got.GetValue(), record.GetValue())
	}
}

func TestDownloadAndImportFileCleansLargeTempFileOnReaderError(t *testing.T) {
	t.Parallel()

	db := openDatastoreTestDB(t)
	store := storage.NewMemoryStore()
	tempDir := t.TempDir()
	key := ChunkKey(99)
	payload := bytes.Repeat([]byte("not-a-valid-netsy-file"), 150000)
	if len(payload) <= 2*1024*1024 {
		t.Fatalf("payload size = %d, want larger than temp-file threshold", len(payload))
	}
	if err := store.Put(context.Background(), key, payload); err != nil {
		t.Fatalf("store.Put() error = %v", err)
	}

	err := DownloadAndImportFile(
		context.Background(),
		slog.Default(),
		db,
		store,
		tempDir,
		key,
		int64(len(payload)),
		pb.FileKind_KIND_CHUNK,
		nil,
	)
	if err == nil {
		t.Fatal("DownloadAndImportFile() error = nil, want invalid datafile error")
	}

	entries, err := os.ReadDir(tempDir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("temp dir contains %d entries after failed import, want 0", len(entries))
	}
}

// openDatastoreTestDB opens a SQLite database suitable for datastore tests.
func openDatastoreTestDB(t *testing.T) localdb.Database {
	t.Helper()

	db := localdb.New(filepath.Join(t.TempDir(), "datastore.sqlite3"))
	if err := db.Connect(); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})

	return db
}

// encodeDatastoreTestFile encodes records into a single Netsy file payload for
// datastore import tests.
func encodeDatastoreTestFile(t *testing.T, kind pb.FileKind, records ...*pb.Record) []byte {
	t.Helper()

	buffer := &bytes.Buffer{}
	writer, err := datafile.NewWriter(bufio.NewWriter(buffer), kind, int64(len(records)), "leader-1")
	if err != nil {
		t.Fatalf("NewWriter() error = %v", err)
	}
	for _, record := range records {
		if err := writer.Write(record); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	return buffer.Bytes()
}
