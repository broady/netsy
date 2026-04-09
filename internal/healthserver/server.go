// Copyright 2026 Nadrama Pty Ltd
// SPDX-License-Identifier: Apache-2.0

package healthserver

import (
	"encoding/json"
	"log/slog"
	"net"
	"net/http"

	"github.com/nadrama-com/netsy/internal/nodestate"
)

// Server serves the /health HTTP endpoint.
type Server struct {
	logger   *slog.Logger
	state    *nodestate.State
	server   *http.Server
	listener net.Listener
}

// New creates a Server bound to addr. Call Start to begin serving.
func New(logger *slog.Logger, addr string, state *nodestate.State) (*Server, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	s := &Server{
		logger:   logger,
		state:    state,
		listener: ln,
		server:   &http.Server{Handler: mux},
	}
	mux.HandleFunc("/health", s.handleHealth)
	return s, nil
}

// Start serves HTTP requests in a new goroutine.
func (s *Server) Start() {
	go func() {
		if err := s.server.Serve(s.listener); err != nil && err != http.ErrServerClosed {
			s.logger.Error("health server error", "error", err)
		}
	}()
}

// Close shuts down the HTTP server gracefully.
func (s *Server) Close() {
	s.server.Close()
}

type healthResponse struct {
	Status string `json:"status"`
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	health := s.state.Health()

	w.Header().Set("Content-Type", "application/json")
	if health != nodestate.HealthHealthy {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	json.NewEncoder(w).Encode(healthResponse{Status: string(health)})
}
