// SPDX-License-Identifier: Apache-2.0

package commonapi

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/netsy-dev/netsy/internal/localdb"
	pb "go.etcd.io/etcd/api/v3/etcdserverpb"
)

func newRangeReproDB(t *testing.T) localdb.Database {
	t.Helper()

	db := localdb.New(filepath.Join(t.TempDir(), "range-repro.sqlite3"))
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

func TestRangeEmptyKeyReproducesPanic(t *testing.T) {
	t.Parallel()

	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatal("Range() did not panic for an empty key")
		}
	}()

	_, _ = Range(newRangeReproDB(t), context.Background(), &pb.RangeRequest{})
}

func TestRangeAllKeysReproducesInvalidSQL(t *testing.T) {
	t.Parallel()

	_, err := Range(newRangeReproDB(t), context.Background(), &pb.RangeRequest{
		Key:      []byte{0},
		RangeEnd: []byte{0},
	})
	if err == nil {
		t.Fatal("Range() error = nil, want invalid SQL error for list-all request")
	}
	if !strings.Contains(err.Error(), "near \")\"") {
		t.Fatalf("Range() error = %v, want WHERE () syntax error", err)
	}
}

func TestRangeMutatesKeyBackingArray(t *testing.T) {
	t.Parallel()

	key := make([]byte, 3, 4)
	copy(key, "abc")
	backing := key[:cap(key)]
	backing[3] = 'x'

	_, err := Range(newRangeReproDB(t), context.Background(), &pb.RangeRequest{Key: key})
	if err != nil {
		t.Fatalf("Range() error = %v", err)
	}

	if backing[3] != 0 {
		t.Fatalf("backing[3] = %q, want appended zero byte to reproduce mutation", backing[3])
	}
}
