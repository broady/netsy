// Copyright 2026 Nadrama Pty Ltd
// SPDX-License-Identifier: Apache-2.0

package bootstrap

import (
	"bufio"
	"context"
	"fmt"
	"os"

	"log/slog"

	"github.com/nadrama-com/netsy/internal/config"
	"github.com/nadrama-com/netsy/internal/datafile"
	"github.com/nadrama-com/netsy/internal/datastore"
	"github.com/nadrama-com/netsy/internal/localdb"
	pb "github.com/nadrama-com/netsy/internal/proto"
	"github.com/nadrama-com/netsy/internal/storage"
)

// importLatestSnapshot imports the newest durable snapshot into an empty local
// database when object storage has one available.
func importLatestSnapshot(
	ctx context.Context,
	logger *slog.Logger,
	db localdb.Database,
	cfg *config.Config,
	snapshotInfo *datastore.LatestSnapshotInfo,
	storageClient storage.ObjectStorage,
) error {
	if snapshotInfo == nil || !snapshotInfo.Found {
		return nil
	}

	logger.Info("database is empty, importing latest snapshot",
		"key", snapshotInfo.Key,
		"revision", snapshotInfo.Revision,
	)

	return downloadAndImportFile(
		ctx,
		logger,
		db,
		storageClient,
		cfg,
		snapshotInfo.Key,
		snapshotInfo.Size,
		pb.FileKind_KIND_SNAPSHOT,
	)
}

// backfillChunksFromRevision replays every chunk file above fromRevision into
// the local database. Idempotent record replay allows this to overlap with
// live replication safely during bootstrap.
func backfillChunksFromRevision(
	ctx context.Context,
	logger *slog.Logger,
	db localdb.Database,
	cfg *config.Config,
	fromRevision int64,
	storageClient storage.ObjectStorage,
) error {
	chunks, err := datastore.ListChunks(ctx, storageClient, fromRevision)
	if err != nil {
		return fmt.Errorf("list chunks: %w", err)
	}

	if len(chunks) == 0 {
		logger.Info("no chunk files found for backfill", "from_revision", fromRevision)
		return nil
	}

	logger.Info("backfilling chunk files",
		"from_revision", fromRevision,
		"count", len(chunks),
	)

	for _, chunk := range chunks {
		if err := downloadAndImportFile(
			ctx,
			logger,
			db,
			storageClient,
			cfg,
			chunk.Key,
			chunk.Size,
			pb.FileKind_KIND_CHUNK,
		); err != nil {
			return fmt.Errorf("import chunk %s: %w", chunk.Key, err)
		}
	}

	return nil
}

// downloadAndImportFile downloads a snapshot or chunk file and imports its
// records into SQLite, cleaning up any temporary files afterwards.
func downloadAndImportFile(
	ctx context.Context,
	logger *slog.Logger,
	db localdb.Database,
	storageClient storage.ObjectStorage,
	cfg *config.Config,
	key string,
	size int64,
	expectedKind pb.FileKind,
) error {
	var tempFiles []string
	defer cleanupTempFiles(logger, tempFiles)

	reader, err := datastore.Download(ctx, storageClient, key, size, cfg.DataDir, &tempFiles)
	if err != nil {
		return fmt.Errorf("download %s: %w", key, err)
	}
	defer reader.Close()

	return importFromReader(logger, db, bufio.NewReader(reader), expectedKind, key)
}

// cleanupTempFiles removes any temporary files created during object-storage
// downloads for bootstrap imports.
func cleanupTempFiles(logger *slog.Logger, tempFiles []string) {
	for _, file := range tempFiles {
		if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
			logger.Warn("failed to remove temporary bootstrap file", "file", file, "error", err)
		}
	}
}

// importFromReader decodes a single Netsy data file and replays every record
// into the local database.
func importFromReader(
	logger *slog.Logger,
	db localdb.Database,
	buffer *bufio.Reader,
	expectedKind pb.FileKind,
	key string,
) error {
	reader, err := datafile.NewReader(buffer, &expectedKind)
	if err != nil {
		return fmt.Errorf("create datafile reader for %s: %w", key, err)
	}

	var recordCount int64
	for i := int64(0); i < reader.Count(); i++ {
		record, err := reader.Read()
		if err != nil {
			return fmt.Errorf("read record %d from %s: %w", i, key, err)
		}
		if _, err := db.ReplicateRecord(record); err != nil {
			return fmt.Errorf("replicate record %d from %s: %w", i, key, err)
		}
		recordCount++
	}

	results, err := reader.Close()
	if err != nil {
		return fmt.Errorf("close datafile reader for %s: %w", key, err)
	}

	logger.Info("imported datafile",
		"key", key,
		"kind", results.Kind,
		"records", recordCount,
		"first_revision", results.FirstRevision,
		"last_revision", results.LastRevision,
	)

	return nil
}
