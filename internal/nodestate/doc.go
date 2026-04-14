// Copyright 2026 Nadrama Pty Ltd
// SPDX-License-Identifier: Apache-2.0

// Package nodestate tracks per-node runtime state: the Health, Elector,
// and Primary state triple, the current Cluster State, and the
// Committed Revision and Compaction Revision used to gate client-visible
// range requests and watches.
package nodestate
