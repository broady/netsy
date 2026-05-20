// SPDX-License-Identifier: Apache-2.0

package watch

import (
	"log/slog"
	"testing"
	"time"

	"github.com/netsy-dev/netsy/internal/proto"
	pb "go.etcd.io/etcd/api/v3/etcdserverpb"
)

func TestIsInRangeEmptyRangeKeyReturnsFalse(t *testing.T) {
	t.Parallel()

	got := isInRange([]byte("record-key"), nil, []byte("z"))
	if got {
		t.Fatal("isInRange() returned true for an empty range key, want false")
	}
}

func TestIsInRangeMutatesRangeKeyBackingArray(t *testing.T) {
	t.Parallel()

	rangeKey := make([]byte, 1, 2)
	rangeKey[0] = 'a'
	backing := rangeKey[:cap(rangeKey)]
	backing[1] = 'x'

	_ = isInRange([]byte("a"), rangeKey, nil)

	if backing[1] != 0 {
		t.Fatalf("backing[1] = %q, want appended zero byte to reproduce mutation", backing[1])
	}
}

func TestDistributeReusesEventPointerAcrossQueuedResponses(t *testing.T) {
	t.Parallel()

	manager := NewManager(slog.Default(), nil)
	watcher := &Watcher{
		id:      1,
		inboxOk: true,
		inboxCh: make(chan pb.WatchResponse, 2),
		watches: map[int64]watchEntry{
			10: {key: []byte("key"), prevKv: true},
			20: {key: []byte("key"), prevKv: false},
		},
		progress: map[int64]bool{},
	}
	manager.Register(watcher)

	manager.Distribute(
		&proto.Record{Revision: 2, Key: []byte("key"), Value: []byte("new")},
		&proto.Record{Revision: 1, Key: []byte("key"), Value: []byte("old")},
	)

	responses := map[int64]pb.WatchResponse{}
	for i := 0; i < 2; i++ {
		select {
		case msg := <-watcher.inboxCh:
			responses[msg.WatchId] = msg
		default:
			t.Fatalf("received %d responses, want 2", i)
		}
	}

	for watchID, msg := range responses {
		gotPrevKV := len(msg.Events) == 1 && msg.Events[0].PrevKv != nil
		wantPrevKV := watchID == 10
		if gotPrevKV != wantPrevKV {
			t.Errorf("watch %d: gotPrevKV=%v, wantPrevKV=%v", watchID, gotPrevKV, wantPrevKV)
		}
	}
}

func TestDistributeBlocksWithFullWatcherInbox(t *testing.T) {
	t.Parallel()

	manager := NewManager(slog.Default(), nil)
	watcher := &Watcher{
		id:      1,
		inboxOk: true,
		inboxCh: make(chan pb.WatchResponse, 1),
		watches: map[int64]watchEntry{
			10: {key: []byte("key"), cancel: func() {}},
		},
		progress: map[int64]bool{},
	}
	manager.Register(watcher)
	// Fill the inbox so the next send would block in the buggy version.
	watcher.inboxCh <- pb.WatchResponse{}

	distributeDone := make(chan struct{})
	go func() {
		manager.Distribute(&proto.Record{Revision: 1, Key: []byte("key")}, nil)
		close(distributeDone)
	}()

	// After the fix, Distribute should return promptly (non-blocking send
	// drops the event when inbox is full).
	select {
	case <-distributeDone:
	case <-time.After(time.Second):
		t.Fatal("Distribute() blocked on a full watcher inbox; expected non-blocking drop")
	}

	// Cleanup should also complete without deadlock.
	cleanupDone := make(chan struct{})
	go func() {
		watcher.Cleanup(manager, slog.Default())
		close(cleanupDone)
	}()

	select {
	case <-cleanupDone:
	case <-time.After(time.Second):
		t.Fatal("Cleanup() blocked after Distribute() completed; expected it to finish promptly")
	}
}
