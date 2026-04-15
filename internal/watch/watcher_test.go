// Copyright 2026 Nadrama Pty Ltd
// SPDX-License-Identifier: Apache-2.0

package watch

import (
	"fmt"
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
