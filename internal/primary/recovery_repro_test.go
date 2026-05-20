// SPDX-License-Identifier: Apache-2.0

package primary

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/netsy-dev/netsy/internal/config"
	"github.com/netsy-dev/netsy/internal/nodestate"
	"github.com/netsy-dev/netsy/internal/proto"
	"github.com/netsy-dev/netsy/internal/storage"
)

// blockingStore blocks every Put until unblocked via the done channel.
type blockingStore struct {
	storage.ObjectStorage
	done chan struct{}
}

func (b *blockingStore) Get(_ context.Context, _ string) ([]byte, string, error) {
	return nil, "", storage.ErrNotFound
}

func (b *blockingStore) Put(ctx context.Context, _ string, _ []byte) error {
	select {
	case <-b.done:
		return errors.New("storage still failing")
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (b *blockingStore) PutIfMatch(ctx context.Context, _ string, _ []byte, _ string) error {
	select {
	case <-b.done:
		return errors.New("storage still failing")
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (b *blockingStore) GetStream(_ context.Context, _ string) (io.ReadCloser, error) {
	return nil, storage.ErrNotFound
}

func (b *blockingStore) PutStream(ctx context.Context, _ string, _ io.Reader, _ int64) error {
	select {
	case <-b.done:
		return errors.New("storage still failing")
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (b *blockingStore) Delete(_ context.Context, _ string) error {
	return nil
}

func (b *blockingStore) List(_ context.Context, _ string) ([]storage.ObjectInfo, error) {
	return nil, nil
}

// countGoroutines returns the number of goroutines whose stack contains substr.
func countGoroutines(substr string) int {
	buf := make([]byte, 1<<20)
	n := runtime.Stack(buf, true)
	return strings.Count(string(buf[:n]), substr)
}

func TestRecoveryGoroutineExitsOnStopServices(t *testing.T) {
	t.Parallel()

	done := make(chan struct{})
	defer close(done) // unblock goroutines at end so they can exit

	store := &blockingStore{done: done}
	state := nodestate.New(slog.Default())
	if err := state.SetPrimary(nodestate.PrimaryStarting); err != nil {
		t.Fatalf("SetPrimary(Starting) error = %v", err)
	}
	if err := state.SetPrimary(nodestate.PrimaryActive); err != nil {
		t.Fatalf("SetPrimary(Active) error = %v", err)
	}

	srv := &Server{
		logger:        slog.Default(),
		storageClient: store,
		state:         state,
		replicas:      NewReplicas(),
		followStreams:  make(map[string]*followSession),
		config: &config.Config{
			NodeConfig: config.NodeConfig{NodeID: "node-a"},
		},
	}

	// Start services so svcCtx is set, then launch recovery goroutine
	srv.StartServices(context.Background())

	before := countGoroutines("startObjectStorageRecovery")

	// Launch recovery goroutine (simulates object storage failure during write)
	srv.startObjectStorageRecovery(
		&proto.Record{Revision: 42, Key: []byte("key")},
		errors.New("upload failed"),
	)

	// Wait for goroutine to be in the retry loop
	time.Sleep(50 * time.Millisecond)

	// Stop services -- this cancels svcCtx, which should cancel the recovery goroutine
	srv.StopServices()

	// Wait for cancellation to propagate
	time.Sleep(100 * time.Millisecond)

	after := countGoroutines("startObjectStorageRecovery")
	leaked := after - before
	if leaked > 0 {
		t.Fatalf("recovery goroutine still alive after StopServices(); "+
			"leaked %d goroutine(s)", leaked)
	}
}
