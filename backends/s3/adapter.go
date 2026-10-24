package s3

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"go.uber.org/zap"

	"github.com/ebogdum/callfs/v2/config"
)

const (
	uploadPartSize      = 5 * 1024 * 1024
	uploadConcurrency   = 3
	headBucketTimeout   = 15 * time.Second
	s3DialTimeout       = 10 * time.Second
	s3TLSHandshake      = 10 * time.Second
	s3ResponseHeaderMax = 60 * time.Second
	s3IdleConnTimeout   = 90 * time.Second
	s3MaxIdlePerHost    = 64
)

func newS3HTTPClient() *http.Client {
	dialer := &net.Dialer{Timeout: s3DialTimeout, KeepAlive: 30 * time.Second}
	return &http.Client{
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           dialer.DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          s3MaxIdlePerHost * 4,
			MaxIdleConnsPerHost:   s3MaxIdlePerHost,
			IdleConnTimeout:       s3IdleConnTimeout,
			TLSHandshakeTimeout:   s3TLSHandshake,
			ExpectContinueTimeout: time.Second,
			ResponseHeaderTimeout: s3ResponseHeaderMax,
		},
	}
}

type S3Adapter struct {
	client               *s3.Client
	uploader             *manager.Uploader
	bucketName           string
	serverSideEncryption string
	acl                  string
	kmsKeyID             string
	logger               *zap.Logger
}

func NewS3Adapter(cfg config.BackendConfig, logger *zap.Logger) (*S3Adapter, error) {
	if cfg.S3BucketName == "" {
		return nil, fmt.Errorf("S3 bucket name is required")
	}

	if "" != cfg.S3Endpoint && strings.HasPrefix(cfg.S3Endpoint, "http://") {
		if !cfg.S3AllowInsecure {
			return nil, fmt.Errorf("S3 endpoint %s uses plaintext HTTP; set s3_allow_insecure to permit it", cfg.S3Endpoint)
		}
		logger.Warn("S3 endpoint uses plaintext HTTP; object data and credentials are sent unencrypted", zap.String("endpoint", cfg.S3Endpoint))
	}

	awsConfig := aws.Config{
		Region: cfg.S3Region,
		Credentials: credentials.NewStaticCredentialsProvider(
			cfg.S3AccessKey,
			cfg.S3SecretKey,
			"",
		),
		HTTPClient: newS3HTTPClient(),
	}

	client := s3.NewFromConfig(awsConfig, func(o *s3.Options) {
		if "" != cfg.S3Endpoint {
			o.BaseEndpoint = aws.String(cfg.S3Endpoint)
			o.UsePathStyle = true
		}
	})

	headCtx, cancel := context.WithTimeout(context.Background(), headBucketTimeout)
	defer cancel()
	if _, err := client.HeadBucket(headCtx, &s3.HeadBucketInput{
		Bucket: aws.String(cfg.S3BucketName),
	}); nil != err {
		return nil, fmt.Errorf("failed to access S3 bucket %s: %w", cfg.S3BucketName, err)
	}

	return &S3Adapter{
		client: client,
		uploader: manager.NewUploader(client, func(u *manager.Uploader) {
			u.PartSize = uploadPartSize
			u.Concurrency = uploadConcurrency
			u.BufferProvider = manager.NewBufferedReadSeekerWriteToPool(uploadPartSize)
		}),
		bucketName:           cfg.S3BucketName,
		serverSideEncryption: cfg.S3ServerSideEncryption,
		acl:                  cfg.S3ACL,
		kmsKeyID:             cfg.S3KMSKeyID,
		logger:               logger,
	}, nil
}

func (a *S3Adapter) Close() error {
	return nil
}

func (a *S3Adapter) pathToKey(path string) string {
	key := strings.TrimPrefix(path, "/")
	return key
}

func (a *S3Adapter) keyToPath(key string) string {
	if key == "" {
		return "/"
	}
	if !strings.HasPrefix(key, "/") {
		key = "/" + key
	}
	return key
}

func isS3NotFound(err error) bool {
	if nil == err {
		return false
	}
	var noSuchKey *types.NoSuchKey
	var notFound *types.NotFound
	return errors.As(err, &noSuchKey) || errors.As(err, &notFound)
}
