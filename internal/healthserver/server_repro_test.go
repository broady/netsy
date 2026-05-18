// SPDX-License-Identifier: Apache-2.0

package healthserver

import (
	"log/slog"
	"testing"

	"github.com/netsy-dev/netsy/internal/nodestate"
)

func TestHTTPServerTimeoutsAreZero(t *testing.T) {
	t.Parallel()

	server, err := New(slog.Default(), "127.0.0.1:0", nodestate.New(slog.Default()), nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(server.Close)

	if server.server.ReadHeaderTimeout != 0 {
		t.Fatalf("ReadHeaderTimeout = %s, want zero to reproduce missing timeout", server.server.ReadHeaderTimeout)
	}
	if server.server.ReadTimeout != 0 {
		t.Fatalf("ReadTimeout = %s, want zero to reproduce missing timeout", server.server.ReadTimeout)
	}
	if server.server.WriteTimeout != 0 {
		t.Fatalf("WriteTimeout = %s, want zero to reproduce missing timeout", server.server.WriteTimeout)
	}
	if server.server.IdleTimeout != 0 {
		t.Fatalf("IdleTimeout = %s, want zero to reproduce missing timeout", server.server.IdleTimeout)
	}
}
