// SPDX-License-Identifier: Apache-2.0

package watch

import (
	"context"
	"database/sql"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	pb "go.etcd.io/etcd/api/v3/etcdserverpb"
	"google.golang.org/grpc/metadata"
)

// racyStream implements pb.Watch_WatchServer with a Send method that
// records concurrent callers. It does NOT synchronize Send internally,
// reproducing the real gRPC ServerStream contract.
type racyStream struct {
	ctx      context.Context
	sendBusy atomic.Bool // true while a Send is in progress
	overlap  atomic.Bool // set to true when concurrent Sends detected
}

func (s *racyStream) Send(_ *pb.WatchResponse) error {
	if !s.sendBusy.CompareAndSwap(false, true) {
		s.overlap.Store(true) // another goroutine is already in Send
	}
	// Simulate a brief write to widen the race window.
	time.Sleep(time.Microsecond)
	s.sendBusy.Store(false)
	return nil
}

func (s *racyStream) Recv() (*pb.WatchRequest, error) {
	// Block forever -- the test drives Send directly.
	<-s.ctx.Done()
	return nil, s.ctx.Err()
}

func (s *racyStream) SetHeader(metadata.MD) error  { return nil }
func (s *racyStream) SendHeader(metadata.MD) error  { return nil }
func (s *racyStream) SetTrailer(metadata.MD)         {}
func (s *racyStream) Context() context.Context       { return s.ctx }
func (s *racyStream) SendMsg(interface{}) error      { return nil }
func (s *racyStream) RecvMsg(interface{}) error      { return nil }

// TestCreateWatchSendRacesWithInboxDispatch shows that CreateWatch
// calls Send on the gRPC stream directly while the inbox dispatch
// goroutine also calls Send. Under -race this detects a data race;
// without -race, the overlap counter demonstrates concurrent access.
func TestCreateWatchSendRacesWithInboxDispatch(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stream := &racyStream{ctx: ctx}
	w := NewWatcher(stream)

	// Start the inbox dispatch goroutine (mirrors etcdapi_watch_watch.go:46-66)
	go func() {
		for msg := range w.inboxCh {
			_ = w.Send(&msg)
		}
	}()

	// Pump events into inbox to keep the dispatch goroutine busy sending.
	pumpDone := make(chan struct{})
	go func() {
		defer close(pumpDone)
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			w.RLock()
			ok := w.inboxOk
			w.RUnlock()
			if !ok {
				return
			}
			select {
			case w.inboxCh <- pb.WatchResponse{WatchId: 999}:
			case <-ctx.Done():
				return
			default:
			}
		}
	}()

	// CreateWatch calls w.client.Send() directly from this goroutine,
	// racing with the inbox dispatch goroutine.
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w.CreateWatch(
				&pb.WatchCreateRequest{Key: []byte("k")},
				10,                          // latestRevision
				0,                           // compactionRevision
				func() int64 { return 0 },   // getAdmissionFloor
				func(rev int64) (int64, bool, sql.NullString, error) {
					return rev, false, sql.NullString{}, nil
				},
				slog.Default(),
			)
		}()
	}
	wg.Wait()

	// Stop the event pump before closing the channel to avoid
	// send-on-closed-channel race.
	cancel()
	<-pumpDone

	w.Lock()
	w.inboxOk = false
	close(w.inboxCh)
	w.Unlock()

	if stream.overlap.Load() {
		t.Fatal("concurrent Send calls detected: the send mutex must serialize all writes to the gRPC stream")
	}
}
