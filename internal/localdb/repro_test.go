// SPDX-License-Identifier: Apache-2.0

package localdb

import (
	"errors"
	"testing"
	"time"

	"github.com/netsy-dev/netsy/internal/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestInsertRecordMutatesInputOnFailedCompare(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	insertReplicated(t, db, &proto.Record{
		Revision:       1,
		Key:            []byte("key"),
		Created:        true,
		CreateRevision: 1,
		Version:        1,
		LeaderId:       "leader",
		CreatedAt:      timestamppb.New(time.Unix(1, 0).UTC()),
	})

	tx, err := db.BeginTx()
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	record := &proto.Record{
		Revision:     2,
		Key:          []byte("key"),
		PrevRevision: 99,
		LeaderId:     "leader",
		Value:        []byte("value"),
	}
	_, err = db.InsertRecord(record, tx)
	if !errors.Is(err, ErrCompareRevisionFailed) {
		t.Fatalf("InsertRecord() error = %v, want %v", err, ErrCompareRevisionFailed)
	}
	if record.CreatedAt == nil {
		t.Fatal("InsertRecord() left CreatedAt nil; input mutation was not reproduced")
	}
}

func TestReplicateRecordMutatesInputOnDuplicateConflict(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	insertReplicated(t, db, &proto.Record{
		Revision:       5,
		Key:            []byte("key"),
		Created:        true,
		CreateRevision: 5,
		Version:        1,
		LeaderId:       "leader",
		Value:          []byte("old"),
		CreatedAt:      timestamppb.New(time.Unix(5, 0).UTC()),
	})

	record := &proto.Record{
		Revision:       5,
		Key:            []byte("key"),
		Created:        true,
		CreateRevision: 5,
		Version:        1,
		LeaderId:       "leader",
		Value:          []byte("new"),
		CreatedAt:      timestamppb.New(time.Unix(5, 0).UTC()),
	}
	_, err := db.ReplicateRecord(record)
	if !errors.Is(err, ErrReplicateConflict) {
		t.Fatalf("ReplicateRecord() error = %v, want %v", err, ErrReplicateConflict)
	}
	if record.ReplicatedAt == nil {
		t.Fatal("ReplicateRecord() left ReplicatedAt nil; input mutation was not reproduced")
	}
}

func TestConnectLeavesSQLitePoolUnbounded(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	if got := db.conn.Stats().MaxOpenConnections; got != 4 {
		t.Fatalf("MaxOpenConnections = %d, want 4 (bounded pool)", got)
	}
}
