// Netsy <https://netsy.dev>
// Copyright 2026 Nadrama Pty Ltd
// SPDX-License-Identifier: Apache-2.0

// Package node implements the Node gRPC service hosted by every Node.
// It handles cluster state pushes from the Elector, node state queries
// during Primary election, and split-brain prevention.
package node
