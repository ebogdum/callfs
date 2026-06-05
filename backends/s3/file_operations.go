package s3

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
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

	putInput := &s3manager.UploadInput{
		Bucket: aws.String(a.bucketName),
		Key:    aws.String(key),
		Body:   reader,
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

	_, err := a.uploader.UploadWithContext(ctx, putInput)
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

// Move relocates an object from oldPath to newPath using a server-side copy
// followed by a delete of the source. S3 has no native rename, but the copy is
// performed entirely within S3 so the object bytes never transit the server.
func (a *S3Adapter) Move(ctx context.Context, oldPath, newPath string) error {
	srcKey := a.pathToKey(oldPath)
	dstKey := a.pathToKey(newPath)

	copyInput := &s3.CopyObjectInput{
		Bucket:     aws.String(a.bucketName),
		CopySource: aws.String(a.bucketName + "/" + srcKey),
		Key:        aws.String(dstKey),
	}
	if a.serverSideEncryption != "" {
		copyInput.ServerSideEncryption = aws.String(a.serverSideEncryption)
		if a.serverSideEncryption == "aws:kms" && a.kmsKeyID != "" {
			copyInput.SSEKMSKeyId = aws.String(a.kmsKeyID)
		}
	}
	if a.acl != "" {
		copyInput.ACL = aws.String(a.acl)
	}

	if _, err := a.client.CopyObjectWithContext(ctx, copyInput); err != nil {
		if isS3NotFound(err) {
			return metadata.ErrNotFound
		}
		return fmt.Errorf("failed to copy object during move: %w", err)
	}

	if _, err := a.client.DeleteObjectWithContext(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(a.bucketName),
		Key:    aws.String(srcKey),
	}); err != nil {
		return fmt.Errorf("failed to delete source object after move: %w", err)
	}

	a.logger.Debug("Object moved in S3",
		zap.String("bucket", a.bucketName),
		zap.String("src_key", srcKey),
		zap.String("dst_key", dstKey))

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
