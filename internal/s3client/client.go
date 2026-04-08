// Copyright 2025 Nadrama Pty Ltd
// SPDX-License-Identifier: Apache-2.0

package s3client

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/nadrama-com/netsy/internal/config"
)

// S3Client wraps AWS S3 operations for Netsy
type S3Client struct {
	client *s3.Client
	config *config.Config
	logger *slog.Logger
}

// FileInfo represents metadata about a file in S3 - used for list operations
type FileInfo struct {
	Key      string
	Size     int64
	Revision int64
}

// New creates a new S3Client with the provided configuration.
// AWS SDK reads AWS_DEFAULT_REGION, AWS_ENDPOINT_URL, AWS_ACCESS_KEY_ID,
// AWS_SECRET_ACCESS_KEY, AWS_SESSION_TOKEN from env automatically via LoadDefaultConfig.
func New(cfg *config.Config, logger *slog.Logger) (*S3Client, error) {
	// Load AWS config - SDK reads region, endpoint, and credentials from env
	awsCfg, err := awsconfig.LoadDefaultConfig(context.TODO())
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Configure credentials with STS AssumeRole if specified
	roleArn := os.Getenv("AWS_ROLE_ARN")
	if roleArn != "" {
		stsClient := sts.NewFromConfig(awsCfg)
		roleSessionName := os.Getenv("AWS_ROLE_SESSION_NAME")
		if roleSessionName == "" {
			roleSessionName = "netsy-session"
		}
		provider := stscreds.NewAssumeRoleProvider(stsClient, roleArn, func(o *stscreds.AssumeRoleOptions) {
			o.RoleSessionName = roleSessionName
		})
		awsCfg.Credentials = aws.NewCredentialsCache(provider)
		logger.Info("Using STS AssumeRole for S3 access", "role", roleArn)
	} else {
		logger.Info("Using default AWS credential chain for S3 access")
	}

	// Create S3 client with path-style addressing if needed
	usePathStyle := os.Getenv("AWS_S3_USE_PATH_STYLE") == "true"
	s3Client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = usePathStyle
	})

	logger.Info("S3Client initialized", "bucket", cfg.Storage.BucketName)

	return &S3Client{
		client: s3Client,
		config: cfg,
		logger: logger,
	}, nil
}

// Client returns the underlying S3 client for direct API access
func (s *S3Client) Client() *s3.Client {
	return s.client
}
