// Copyright 2026 Nadrama Pty Ltd
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/nadrama-com/netsy/internal/storage"
)

// Download retrieves an object from storage, using a temp file for large objects
// to avoid holding everything in memory
func Download(ctx context.Context, store storage.ObjectStorage, key string, size int64, dataDir string, tempFiles *[]string) (io.ReadCloser, error) {
	const maxMemorySize = 2 * 1024 * 1024 // 2MB

	reader, err := store.GetStream(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("failed to download file: %w", err)
	}

	if size <= maxMemorySize {
		return reader, nil
	}

	// For large files, write to a temp file to avoid holding everything in memory
	var prefix string
	if strings.Contains(key, "snapshots/") {
		prefix = "snapshot_"
	} else {
		prefix = "chunk_"
	}

	tempFile, err := os.CreateTemp(dataDir, prefix+"*.netsy")
	if err != nil {
		reader.Close()
		return nil, fmt.Errorf("failed to create temporary file: %w", err)
	}
	tempPath := tempFile.Name()
	*tempFiles = append(*tempFiles, tempPath)

	if _, err := io.Copy(tempFile, reader); err != nil {
		tempFile.Close()
		reader.Close()
		return nil, fmt.Errorf("failed to write to temporary file: %w", err)
	}
	reader.Close()
	tempFile.Close()

	readFile, err := os.Open(tempPath)
	if err != nil {
		return nil, fmt.Errorf("failed to reopen downloaded file: %w", err)
	}
	return readFile, nil
}
