// SPDX-License-Identifier: Apache-2.0

package elector

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/netsy-dev/netsy/internal/nodestate"
	"github.com/netsy-dev/netsy/internal/storage"
)

type failingBootstrapStore struct{}

func (f failingBootstrapStore) Get(context.Context, string) ([]byte, string, error) {
	return nil, "", errors.New("storage unavailable")
}

func (f failingBootstrapStore) Put(context.Context, string, []byte) error {
	return errors.New("storage unavailable")
}

func (f failingBootstrapStore) PutIfMatch(context.Context, string, []byte, string) error {
	return errors.New("storage unavailable")
}

func (f failingBootstrapStore) GetStream(context.Context, string) (io.ReadCloser, error) {
	return nil, errors.New("storage unavailable")
}

func (f failingBootstrapStore) PutStream(context.Context, string, io.Reader, int64) error {
	return errors.New("storage unavailable")
}

func (f failingBootstrapStore) Delete(context.Context, string) error {
	return errors.New("storage unavailable")
}

func (f failingBootstrapStore) List(context.Context, string) ([]storage.ObjectInfo, error) {
	return nil, errors.New("storage unavailable")
}

func TestOnAcquireLeadershipReturnsNilAfterBootstrapFailure(t *testing.T) {
	t.Parallel()

	state := nodestate.New(slog.Default())
	runner := &Runner{
		logger:   slog.Default(),
		nodeID:   "node-a",
		peerAddr: "127.0.0.1:2381",
		state:    state,
		server: NewServer(
			slog.Default(),
			"cluster-a",
			failingBootstrapStore{},
			state,
			0,
			0,
			0,
			"node-a",
			1,
			nil,
			0,
			0,
			nil,
			nil,
			nil,
		),
	}

	if err := runner.onAcquireLeadership(); err != nil {
		t.Fatalf("onAcquireLeadership() error = %v, want nil to reproduce async bootstrap failure", err)
	}
	t.Cleanup(func() {
		if err := runner.onLoseLeadership(); err != nil {
			t.Fatalf("onLoseLeadership() error = %v", err)
		}
	})

	time.Sleep(50 * time.Millisecond)

	if runner.server.nodeMap.Ready() {
		t.Fatal("node map became ready; bootstrap failure was not reproduced")
	}
	if got := state.Elector(); got != nodestate.ElectorLeader {
		t.Fatalf("Elector() = %s, want %s to reproduce held leadership after bootstrap failure", got, nodestate.ElectorLeader)
	}
}
