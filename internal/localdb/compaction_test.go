// Copyright 2026 Nadrama Pty Ltd
// SPDX-License-Identifier: Apache-2.0

package localdb

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/nadrama-com/netsy/internal/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestDeriveCompactionRevision(t *testing.T) {
	db := openTestDB(t)

	insertReplicated(t, db, &proto.Record{
		Revision:       1,
		Key:            []byte("a"),
		Created:        true,
		Version:        1,
		CreateRevision: 1,
		CreatedAt:      timestamppb.New(time.Unix(1, 0).UTC()),
		CompactedAt:    timestamppb.New(time.Unix(10, 0).UTC()),
		LeaderId:       "leader-1",
	})
	insertReplicated(t, db, &proto.Record{
		Revision:       2,
		Key:            []byte("b"),
		Created:        true,
		Version:        1,
		CreateRevision: 2,
		CreatedAt:      timestamppb.New(time.Unix(2, 0).UTC()),
		CompactedAt:    timestamppb.New(time.Unix(11, 0).UTC()),
		LeaderId:       "leader-1",
	})
	insertReplicated(t, db, &proto.Record{
		Revision:       3,
		Key:            []byte("c"),
		Created:        true,
		Version:        1,
		CreateRevision: 3,
		CreatedAt:      timestamppb.New(time.Unix(3, 0).UTC()),
		LeaderId:       "leader-1",
	})

	got, err := db.DeriveCompactionRevision()
	if err != nil {
		t.Fatalf("DeriveCompactionRevision() error = %v", err)
	}
	if got != 2 {
		t.Fatalf("DeriveCompactionRevision() = %d, want 2", got)
	}
}

func TestDeriveCompactionRevisionAllCompacted(t *testing.T) {
	db := openTestDB(t)

	insertReplicated(t, db, &proto.Record{
		Revision:       1,
		Key:            []byte("a"),
		Created:        true,
		Version:        1,
		CreateRevision: 1,
		CreatedAt:      timestamppb.New(time.Unix(1, 0).UTC()),
		CompactedAt:    timestamppb.New(time.Unix(10, 0).UTC()),
		LeaderId:       "leader-1",
	})
	insertReplicated(t, db, &proto.Record{
		Revision:       2,
		Key:            []byte("b"),
		Created:        true,
		Version:        1,
		CreateRevision: 2,
		CreatedAt:      timestamppb.New(time.Unix(2, 0).UTC()),
		CompactedAt:    timestamppb.New(time.Unix(11, 0).UTC()),
		LeaderId:       "leader-1",
	})

	got, err := db.DeriveCompactionRevision()
	if err != nil {
		t.Fatalf("DeriveCompactionRevision() error = %v", err)
	}
	if got != 2 {
		t.Fatalf("DeriveCompactionRevision() = %d, want 2", got)
	}
}

func TestPersistCompactionRevisionIdempotent(t *testing.T) {
	db := openTestDB(t)

	if err := db.PersistCompactionRevision(12); err != nil {
		t.Fatalf("PersistCompactionRevision(12) first error = %v", err)
	}
	if err := db.PersistCompactionRevision(12); err != nil {
		t.Fatalf("PersistCompactionRevision(12) second error = %v", err)
	}

	got, err := db.LatestCompactionRevision()
	if err != nil {
		t.Fatalf("LatestCompactionRevision() error = %v", err)
	}
	if got != 12 {
		t.Fatalf("LatestCompactionRevision() = %d, want 12", got)
	}
}

func TestReplicateRecordDuplicateIsIdempotentAndPreservesCompactedAt(t *testing.T) {
	db := openTestDB(t)

	record := &proto.Record{
		Revision:       7,
		Key:            []byte("key"),
		Created:        true,
		Version:        1,
		CreateRevision: 7,
		CreatedAt:      timestamppb.New(time.Unix(7, 0).UTC()),
		CompactedAt:    timestamppb.New(time.Unix(17, 0).UTC()),
		LeaderId:       "leader-1",
	}

	inserted, err := db.ReplicateRecord(record)
	if err != nil {
		t.Fatalf("first ReplicateRecord() error = %v", err)
	}
	if inserted.GetCompactedAt() == nil {
		t.Fatal("first ReplicateRecord() did not persist compacted_at")
	}

	duplicated, err := db.ReplicateRecord(record)
	if err != nil {
		t.Fatalf("second ReplicateRecord() error = %v", err)
	}
	if duplicated.GetRevision() != record.GetRevision() {
		t.Fatalf("duplicate ReplicateRecord() revision = %d, want %d", duplicated.GetRevision(), record.GetRevision())
	}
	if duplicated.GetCompactedAt() == nil || !duplicated.GetCompactedAt().AsTime().Equal(record.GetCompactedAt().AsTime()) {
		t.Fatal("duplicate ReplicateRecord() lost compacted_at")
	}

	count, err := db.RecordCount()
	if err != nil {
		t.Fatalf("RecordCount() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("RecordCount() = %d, want 1", count)
	}
}

func TestReplicateRecordDuplicateMismatchFails(t *testing.T) {
	db := openTestDB(t)

	insertReplicated(t, db, &proto.Record{
		Revision:       9,
		Key:            []byte("key"),
		Created:        true,
		Version:        1,
		CreateRevision: 9,
		CreatedAt:      timestamppb.New(time.Unix(9, 0).UTC()),
		LeaderId:       "leader-1",
		Value:          []byte("value-a"),
	})

	_, err := db.ReplicateRecord(&proto.Record{
		Revision:       9,
		Key:            []byte("key"),
		Created:        true,
		Version:        1,
		CreateRevision: 9,
		CreatedAt:      timestamppb.New(time.Unix(9, 0).UTC()),
		LeaderId:       "leader-1",
		Value:          []byte("value-b"),
	})
	if err == nil {
		t.Fatal("ReplicateRecord() mismatch duplicate succeeded, want error")
	}
}

func openTestDB(t *testing.T) *database {
	t.Helper()

	db := New(filepath.Join(t.TempDir(), "test.sqlite3"))
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

func insertReplicated(t *testing.T, db *database, record *proto.Record) {
	t.Helper()

	if _, err := db.ReplicateRecord(record); err != nil {
		t.Fatalf("ReplicateRecord(%d) error = %v", record.GetRevision(), err)
	}
}
