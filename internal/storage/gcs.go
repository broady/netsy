// Copyright 2026 Nadrama Pty Ltd
// SPDX-License-Identifier: Apache-2.0

package storage

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	gcsstorage "cloud.google.com/go/storage"
	"google.golang.org/api/iterator"

	"github.com/nadrama-com/netsy/internal/config"
)

// gcsProvider implements ObjectStorage for Google Cloud Storage
type gcsProvider struct {
	client *gcsstorage.Client
	config *config.Config
	logger *slog.Logger
}

// newGCSProvider creates a new GCS provider with the provided configuration.
// Authentication uses Application Default Credentials or GOOGLE_APPLICATION_CREDENTIALS.
func newGCSProvider(cfg *config.Config, logger *slog.Logger) (*gcsProvider, error) {
	client, err := gcsstorage.NewClient(context.TODO())
	if err != nil {
		return nil, fmt.Errorf("failed to create GCS client: %w", err)
	}

	logger.Info("GCS storage provider initialized", "bucket", cfg.Storage.BucketName)

	return &gcsProvider{
		client: client,
		config: cfg,
		logger: logger,
	}, nil
}

// Put writes data to the given key
func (p *gcsProvider) Put(ctx context.Context, key string, data io.Reader, size int64) error {
	obj := p.client.Bucket(p.config.Storage.BucketName).Object(key)
	w := obj.NewWriter(ctx)
	w.StorageClass = p.config.Storage.Class

	if p.config.Storage.Encryption == "customer-managed" && p.config.Storage.KMSKeyID != "" {
		w.KMSKeyName = p.config.Storage.KMSKeyID
	}

	if _, err := io.Copy(w, data); err != nil {
		w.Close()
		return fmt.Errorf("failed to write data to GCS: %w", err)
	}

	if err := w.Close(); err != nil {
		return fmt.Errorf("failed to close GCS writer: %w", err)
	}

	p.logger.Debug("object uploaded to GCS", "key", key, "bucket", p.config.Storage.BucketName)
	return nil
}

// PutIfMatch writes data conditionally
func (p *gcsProvider) PutIfMatch(ctx context.Context, key string, data io.Reader, size int64, etag string) error {
	obj := p.client.Bucket(p.config.Storage.BucketName).Object(key)

	if etag == "" {
		obj = obj.If(gcsstorage.Conditions{DoesNotExist: true})
	} else {
		var generation int64
		if _, err := fmt.Sscanf(etag, "%d", &generation); err != nil {
			return fmt.Errorf("failed to parse etag as generation: %w", err)
		}
		obj = obj.If(gcsstorage.Conditions{GenerationMatch: generation})
	}

	w := obj.NewWriter(ctx)
	w.StorageClass = p.config.Storage.Class

	if p.config.Storage.Encryption == "customer-managed" && p.config.Storage.KMSKeyID != "" {
		w.KMSKeyName = p.config.Storage.KMSKeyID
	}

	if _, err := io.Copy(w, data); err != nil {
		w.Close()
		return fmt.Errorf("failed to write data to GCS: %w", err)
	}

	if err := w.Close(); err != nil {
		return fmt.Errorf("failed to close GCS writer: %w", err)
	}

	p.logger.Debug("conditional object uploaded to GCS", "key", key, "bucket", p.config.Storage.BucketName)
	return nil
}

// Get returns a reader for the object at the given key
func (p *gcsProvider) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	reader, err := p.client.Bucket(p.config.Storage.BucketName).Object(key).NewReader(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get object from GCS: %w", err)
	}
	return reader, nil
}

// Delete removes the object at the given key
func (p *gcsProvider) Delete(ctx context.Context, key string) error {
	if err := p.client.Bucket(p.config.Storage.BucketName).Object(key).Delete(ctx); err != nil {
		return fmt.Errorf("failed to delete object from GCS: %w", err)
	}

	p.logger.Debug("object deleted from GCS", "key", key, "bucket", p.config.Storage.BucketName)
	return nil
}

// List returns all object keys matching the given prefix
func (p *gcsProvider) List(ctx context.Context, prefix string) ([]ObjectInfo, error) {
	var objects []ObjectInfo
	it := p.client.Bucket(p.config.Storage.BucketName).Objects(ctx, &gcsstorage.Query{Prefix: prefix})
	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to list objects from GCS: %w", err)
		}

		objects = append(objects, ObjectInfo{
			Key:  attrs.Name,
			Size: attrs.Size,
		})
	}

	return objects, nil
}
