// Copyright 2025 Nadrama Pty Ltd
// SPDX-License-Identifier: Apache-2.0

package s3client

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// DeleteFile deletes a file from S3
func (s *S3Client) DeleteFile(ctx context.Context, key string) error {
	// Prepare S3 key with prefix
	s3Key := key
	if s.config.Storage.KeyPrefix != "" {
		s3Key = s.config.Storage.KeyPrefix + "/" + key
	}

	// Prepare delete object input
	bucketName := s.config.Storage.BucketName
	input := &s3.DeleteObjectInput{
		Bucket: &bucketName,
		Key:    &s3Key,
	}

	// Delete from S3
	_, err := s.client.DeleteObject(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to delete file from S3: %w", err)
	}

	s.logger.Debug("file deleted from S3", "key", s3Key, "bucket", s.config.Storage.BucketName)
	return nil
}
