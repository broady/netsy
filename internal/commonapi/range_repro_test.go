// SPDX-License-Identifier: Apache-2.0

package commonapi

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/netsy-dev/netsy/internal/localdb"
	pb "go.etcd.io/etcd/api/v3/etcdserverpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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

	_, err := Range(newRangeReproDB(t), context.Background(), &pb.RangeRequest{})
	if err == nil {
		t.Fatal("Range() error = nil, want InvalidArgument error for empty key")
	}
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("Range() error code = %v, want InvalidArgument", status.Code(err))
	}
}

func TestRangeAllKeysReproducesInvalidSQL(t *testing.T) {
	t.Parallel()

	resp, err := Range(newRangeReproDB(t), context.Background(), &pb.RangeRequest{
		Key:      []byte{0},
		RangeEnd: []byte{0},
	})
	if err != nil {
		t.Fatalf("Range() error = %v, want nil for list-all request", err)
	}
	if resp.Count < 0 {
		t.Fatalf("Range() Count = %d, want >= 0", resp.Count)
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
