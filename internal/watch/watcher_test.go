// Netsy <https://netsy.dev>
// Copyright 2026 Nadrama Pty Ltd
// SPDX-License-Identifier: Apache-2.0

package watch

import (
	"fmt"
	"log/slog"
	"testing"

	"github.com/nadrama-com/netsy/internal/proto"
)

func TestIsWatchMatch(t *testing.T) {
	tests := []struct {
		w      watchEntry
		record *proto.Record
		expect bool
	}{
		// different key
		{watchEntry{key: []byte("1")}, &proto.Record{Key: []byte("")}, false},
		{watchEntry{key: []byte("1")}, &proto.Record{Key: []byte("")}, false},
		{watchEntry{key: []byte("1")}, &proto.Record{Key: []byte("")}, false},
		// exact match
		{watchEntry{key: []byte("1")}, &proto.Record{Key: []byte("1")}, true},
		{watchEntry{key: []byte("1")}, &proto.Record{Key: []byte("1")}, true},
		{watchEntry{key: []byte("1")}, &proto.Record{Key: []byte("1")}, true},
		// 1 inside range 1-3
		{watchEntry{key: []byte("1"), rangeEnd: []byte("3")}, &proto.Record{Key: []byte("1")}, true},
		{watchEntry{key: []byte("1"), rangeEnd: []byte("3")}, &proto.Record{Key: []byte("1")}, true},
		{watchEntry{key: []byte("1"), rangeEnd: []byte("3")}, &proto.Record{Key: []byte("1")}, true},
		// 2 inside range 1-3
		{watchEntry{key: []byte("1"), rangeEnd: []byte("3")}, &proto.Record{Key: []byte("2")}, true},
		{watchEntry{key: []byte("1"), rangeEnd: []byte("3")}, &proto.Record{Key: []byte("2")}, true},
		{watchEntry{key: []byte("1"), rangeEnd: []byte("3")}, &proto.Record{Key: []byte("2")}, true},
		// 1 prefix match (range 1-2 triggers prefix match)
		{watchEntry{key: []byte("1"), rangeEnd: []byte("2")}, &proto.Record{Key: []byte("1")}, true},
		{watchEntry{key: []byte("1"), rangeEnd: []byte("2")}, &proto.Record{Key: []byte("1")}, true},
		{watchEntry{key: []byte("1"), rangeEnd: []byte("2")}, &proto.Record{Key: []byte("1")}, true},
	}

	for i, test := range tests {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			result := isWatchMatch(test.w, test.record)
			if result != test.expect {
				t.Errorf("isWatchMatch(%+v, %+v)\n= %t\nwant %t", test.w, test.record, result, test.expect)
			}
		})
	}
}

func TestSetWatchAdmissionFloorNoWatches(t *testing.T) {
	m := NewManager(slog.Default(), nil)

	if err := m.SetWatchAdmissionFloor(50); err != nil {
		t.Fatalf("SetWatchAdmissionFloor(50) error = %v", err)
	}
	if got := m.WatchAdmissionFloor(); got != 50 {
		t.Fatalf("WatchAdmissionFloor() = %d, want 50", got)
	}
}

func TestSetWatchAdmissionFloorRejectsWhenWatchBelow(t *testing.T) {
	m := NewManager(slog.Default(), nil)

	// Simulate a watcher with a watch at startRevision 30.
	w := &Watcher{
		id:       1,
		watches:  map[int64]watchEntry{1: {startRevision: 30}},
		progress: map[int64]bool{},
	}
	m.Register(w)

	err := m.SetWatchAdmissionFloor(50)
	if err == nil {
		t.Fatal("SetWatchAdmissionFloor(50) should fail with active watch at revision 30")
	}

	// Floor should have been rolled back.
	if got := m.WatchAdmissionFloor(); got != 0 {
		t.Fatalf("WatchAdmissionFloor() = %d, want 0 after rollback", got)
	}
}

func TestSetWatchAdmissionFloorAcceptsWhenWatchAbove(t *testing.T) {
	m := NewManager(slog.Default(), nil)

	w := &Watcher{
		id:       1,
		watches:  map[int64]watchEntry{1: {startRevision: 80}},
		progress: map[int64]bool{},
	}
	m.Register(w)

	if err := m.SetWatchAdmissionFloor(50); err != nil {
		t.Fatalf("SetWatchAdmissionFloor(50) error = %v", err)
	}
	if got := m.WatchAdmissionFloor(); got != 50 {
		t.Fatalf("WatchAdmissionFloor() = %d, want 50", got)
	}
}

func TestSetWatchAdmissionFloorOverwritesPrevious(t *testing.T) {
	m := NewManager(slog.Default(), nil)

	if err := m.SetWatchAdmissionFloor(50); err != nil {
		t.Fatalf("SetWatchAdmissionFloor(50) error = %v", err)
	}

	// A second set with a lower value (rollback) should overwrite.
	if err := m.SetWatchAdmissionFloor(30); err != nil {
		t.Fatalf("SetWatchAdmissionFloor(30) error = %v", err)
	}
	if got := m.WatchAdmissionFloor(); got != 30 {
		t.Fatalf("WatchAdmissionFloor() = %d, want 30", got)
	}
}
