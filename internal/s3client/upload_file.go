// Copyright 2025 Nadrama Pty Ltd
// SPDX-License-Identifier: Apache-2.0

package s3client

import (
	"context"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// UploadFile uploads a local file to S3
func (s *S3Client) UploadFile(ctx context.Context, key, filePath string) error {
	// Open local file
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %w", filePath, err)
	}
	defer file.Close()

	// Get file info for content length
	fileInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to get file info: %w", err)
	}

	// Prepare S3 key with prefix
	s3Key := key
	if s.config.Storage.KeyPrefix != "" {
		s3Key = s.config.Storage.KeyPrefix + "/" + key
	}

	// Prepare put object input
	bucketName := s.config.Storage.BucketName
	storageClass := s.config.Storage.Class
	input := &s3.PutObjectInput{
		Bucket:       &bucketName,
		Key:          &s3Key,
		Body:         file,
		ContentLength: aws.Int64(fileInfo.Size()),
		StorageClass: types.StorageClass(storageClass),
	}

	// Set server-side encryption
	if s.config.Storage.Encryption == "customer-managed" {
		input.ServerSideEncryption = types.ServerSideEncryptionAwsKms
		if s.config.Storage.KMSKeyID != "" {
			kmsKeyID := s.config.Storage.KMSKeyID
			input.SSEKMSKeyId = &kmsKeyID
		}
	} else {
		input.ServerSideEncryption = types.ServerSideEncryptionAes256
	}

	// Upload to S3
	s.logger.Debug("uploading to S3", "bucket", s.config.Storage.BucketName, "key", s3Key, "size", fileInfo.Size())
	_, err = s.client.PutObject(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to upload file to S3: %w", err)
	}

	s.logger.Info("file uploaded to S3", "key", s3Key, "bucket", s.config.Storage.BucketName, "size", fileInfo.Size())
	return nil
}
