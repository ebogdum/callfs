package s3

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"go.uber.org/zap"

	"github.com/ebogdum/callfs/metadata"
)

// Open opens a file for reading
func (a *S3Adapter) Open(ctx context.Context, path string) (io.ReadCloser, error) {
	key := a.pathToKey(path)

	result, err := a.client.GetObjectWithContext(ctx, &s3.GetObjectInput{
		Bucket: aws.String(a.bucketName),
		Key:    aws.String(key),
	})

	if err != nil {
		if isS3NotFound(err) {
			return nil, metadata.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get object from S3: %w", err)
	}

	a.logger.Debug("File opened from S3",
		zap.String("bucket", a.bucketName),
		zap.String("key", key))

	return result.Body, nil
}

// Create creates a new file
func (a *S3Adapter) Create(ctx context.Context, path string, reader io.Reader, size int64) error {
	key := a.pathToKey(path)

	// Read data into buffer for upload
	data, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("failed to read data: %w", err)
	}

	putInput := &s3.PutObjectInput{
		Bucket: aws.String(a.bucketName),
		Key:    aws.String(key),
		Body:   bytes.NewReader(data),
	}

	// Set server-side encryption if configured
	if a.serverSideEncryption != "" {
		putInput.ServerSideEncryption = aws.String(a.serverSideEncryption)
		if a.serverSideEncryption == "aws:kms" && a.kmsKeyID != "" {
			putInput.SSEKMSKeyId = aws.String(a.kmsKeyID)
		}
	}

	// Set ACL if configured
	if a.acl != "" {
		putInput.ACL = aws.String(a.acl)
	}

	// Set content type based on file extension
	if contentType := getContentType(path); contentType != "" {
		putInput.ContentType = aws.String(contentType)
	}

	_, err = a.client.PutObjectWithContext(ctx, putInput)
	if err != nil {
		return fmt.Errorf("failed to put object to S3: %w", err)
	}

	a.logger.Debug("File created in S3",
		zap.String("bucket", a.bucketName),
		zap.String("key", key),
		zap.Int64("size", size))

	return nil
}

// Update updates an existing file
func (a *S3Adapter) Update(ctx context.Context, path string, reader io.Reader, size int64) error {
	// For S3, update is the same as create
	return a.Create(ctx, path, reader, size)
}

// Delete removes a file or directory
func (a *S3Adapter) Delete(ctx context.Context, path string) error {
	key := a.pathToKey(path)

	_, err := a.client.DeleteObjectWithContext(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(a.bucketName),
		Key:    aws.String(key),
	})

	if err != nil {
		return fmt.Errorf("failed to delete object from S3: %w", err)
	}

	a.logger.Debug("File deleted from S3",
		zap.String("bucket", a.bucketName),
		zap.String("key", key))

	return nil
}

// Stat gets file information
func (a *S3Adapter) Stat(ctx context.Context, path string) (*metadata.Metadata, error) {
	key := a.pathToKey(path)

	result, err := a.client.HeadObjectWithContext(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(a.bucketName),
		Key:    aws.String(key),
	})

	if err != nil {
		if isS3NotFound(err) {
			return nil, metadata.ErrNotFound
		}
		return nil, fmt.Errorf("failed to stat object in S3: %w", err)
	}

	md := &metadata.Metadata{
		Name:        filepath.Base(path),
		Path:        "/" + key, // Ensure leading slash
		Type:        "file",
		Size:        *result.ContentLength,
		Mode:        "0644",
		UID:         1000,
		GID:         1000,
		BackendType: "s3",
	}

	if result.LastModified != nil {
		md.MTime = *result.LastModified
		md.ATime = *result.LastModified
		md.CTime = *result.LastModified
	}

	return md, nil
}

// getContentType returns the MIME type based on file extension
func getContentType(path string) string {
	ext := filepath.Ext(path)
	switch strings.ToLower(ext) {
	case ".html", ".htm":
		return "text/html"
	case ".css":
		return "text/css"
	case ".js":
		return "application/javascript"
	case ".json":
		return "application/json"
	case ".xml":
		return "application/xml"
	case ".pdf":
		return "application/pdf"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".svg":
		return "image/svg+xml"
	case ".txt":
		return "text/plain"
	case ".md":
		return "text/markdown"
	default:
		return "application/octet-stream"
	}
}
