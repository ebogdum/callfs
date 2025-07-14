package s3

import (
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"go.uber.org/zap"

	"github.com/ebogdum/callfs/config"
)

// S3Adapter implements the backends.Storage interface for AWS S3
type S3Adapter struct {
	client               *s3.S3
	bucketName           string
	serverSideEncryption string
	acl                  string
	kmsKeyID             string
	logger               *zap.Logger
}

// NewS3Adapter creates a new S3 storage adapter
func NewS3Adapter(cfg config.BackendConfig, logger *zap.Logger) (*S3Adapter, error) {
	if cfg.S3BucketName == "" {
		return nil, fmt.Errorf("S3 bucket name is required")
	}

	// Create AWS session
	awsConfig := &aws.Config{
		Region: aws.String(cfg.S3Region),
		Credentials: credentials.NewStaticCredentials(
			cfg.S3AccessKey,
			cfg.S3SecretKey,
			"",
		),
		DisableSSL: aws.Bool(true), // MinIO typically uses HTTP
	}

	// Set custom endpoint if provided (for MinIO compatibility)
	if cfg.S3Endpoint != "" {
		awsConfig.Endpoint = aws.String(cfg.S3Endpoint)
		awsConfig.S3ForcePathStyle = aws.Bool(true)              // Required for MinIO
		awsConfig.S3DisableContentMD5Validation = aws.Bool(true) // Disable MD5 for MinIO
	}

	sess, err := session.NewSession(awsConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create AWS session: %w", err)
	}

	client := s3.New(sess)

	// Verify bucket access
	_, err = client.HeadBucket(&s3.HeadBucketInput{
		Bucket: aws.String(cfg.S3BucketName),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to access S3 bucket %s: %w", cfg.S3BucketName, err)
	}

	return &S3Adapter{
		client:               client,
		bucketName:           cfg.S3BucketName,
		serverSideEncryption: cfg.S3ServerSideEncryption,
		acl:                  cfg.S3ACL,
		kmsKeyID:             cfg.S3KMSKeyID,
		logger:               logger,
	}, nil
}

// Close closes any resources used by the S3 adapter
func (a *S3Adapter) Close() error {
	// No resources to close for S3
	return nil
}

// pathToKey converts a filesystem path to an S3 key
func (a *S3Adapter) pathToKey(path string) string {
	// Remove leading slash and normalize
	key := strings.TrimPrefix(path, "/")
	return key
}

// keyToPath converts an S3 key to a filesystem path
func (a *S3Adapter) keyToPath(key string) string {
	if key == "" {
		return "/"
	}
	if !strings.HasPrefix(key, "/") {
		key = "/" + key
	}
	return key
}

// isS3NotFound checks if an error indicates the object was not found
func isS3NotFound(err error) bool {
	return strings.Contains(err.Error(), "NoSuchKey") || strings.Contains(err.Error(), "NotFound")
}
