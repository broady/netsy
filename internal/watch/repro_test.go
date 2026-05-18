// SPDX-License-Identifier: Apache-2.0

package watch

import (
	"log/slog"
	"testing"
	"time"

	"github.com/netsy-dev/netsy/internal/proto"
	pb "go.etcd.io/etcd/api/v3/etcdserverpb"
)

func TestIsInRangeEmptyRangeKeyReproducesPanic(t *testing.T) {
	t.Parallel()

	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatal("isInRange() did not panic for an empty range key")
		}
	}()

	_ = isInRange([]byte("record-key"), nil, []byte("z"))
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

	mismatches := 0
	for watchID, msg := range responses {
		gotPrevKV := len(msg.Events) == 1 && msg.Events[0].PrevKv != nil
		wantPrevKV := watchID == 10
		if gotPrevKV != wantPrevKV {
			mismatches++
		}
	}
	if mismatches == 0 {
		t.Fatal("all queued responses retained the expected PrevKv state; shared pointer corruption was not reproduced")
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
	watcher.inboxCh <- pb.WatchResponse{}

	distributeDone := make(chan struct{})
	go func() {
		manager.Distribute(&proto.Record{Revision: 1, Key: []byte("key")}, nil)
		close(distributeDone)
	}()

	select {
	case <-distributeDone:
		t.Fatal("Distribute() returned even though the watcher inbox was full")
	case <-time.After(100 * time.Millisecond):
	}

	cleanupDone := make(chan struct{})
	go func() {
		watcher.Cleanup(manager, slog.Default())
		close(cleanupDone)
	}()

	select {
	case <-cleanupDone:
		t.Fatal("Cleanup() returned while Distribute() held watcher and manager read locks")
	case <-time.After(100 * time.Millisecond):
	}

	<-watcher.inboxCh

	select {
	case <-distributeDone:
	case <-time.After(time.Second):
		t.Fatal("Distribute() did not unblock after inbox space was made available")
	}
	select {
	case <-cleanupDone:
	case <-time.After(time.Second):
		t.Fatal("Cleanup() did not unblock after Distribute() completed")
	}
}
