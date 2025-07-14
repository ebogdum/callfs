package s3

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"go.uber.org/zap"

	"github.com/ebogdum/callfs/metadata"
)

// ListDirectory lists the contents of a directory
func (a *S3Adapter) ListDirectory(ctx context.Context, path string) ([]*metadata.Metadata, error) {
	// Normalize the path to be a prefix
	prefix := strings.TrimPrefix(path, "/")
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	input := &s3.ListObjectsV2Input{
		Bucket:    aws.String(a.bucketName),
		Prefix:    aws.String(prefix),
		Delimiter: aws.String("/"),
	}

	var results []*metadata.Metadata

	for {
		result, err := a.client.ListObjectsV2WithContext(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("failed to list objects in S3: %w", err)
		}

		// Process directory objects (common prefixes)
		for _, commonPrefix := range result.CommonPrefixes {
			if commonPrefix.Prefix == nil {
				continue
			}

			// Remove prefix and trailing slash to get directory name
			dirName := strings.TrimSuffix(strings.TrimPrefix(*commonPrefix.Prefix, prefix), "/")
			if dirName == "" {
				continue
			}

			md := &metadata.Metadata{
				Name:        dirName,
				Path:        a.keyToPath(strings.TrimSuffix(*commonPrefix.Prefix, "/")),
				Type:        "directory",
				Size:        0,
				Mode:        "0755",
				UID:         1000,
				GID:         1000,
				BackendType: "s3",
				ATime:       time.Now(),
				MTime:       time.Now(),
				CTime:       time.Now(),
			}

			results = append(results, md)
		}

		// Process file objects
		for _, object := range result.Contents {
			if object.Key == nil {
				continue
			}

			// Skip if this is the directory marker itself
			if strings.HasSuffix(*object.Key, "/") {
				continue
			}

			// Get just the filename (remove prefix)
			fileName := strings.TrimPrefix(*object.Key, prefix)
			if fileName == "" || strings.Contains(fileName, "/") {
				continue // Skip if it's in a subdirectory
			}

			md := &metadata.Metadata{
				Name:        fileName,
				Path:        a.keyToPath(*object.Key),
				Type:        "file",
				Size:        *object.Size,
				Mode:        "0644",
				UID:         1000,
				GID:         1000,
				BackendType: "s3",
			}

			if object.LastModified != nil {
				md.MTime = *object.LastModified
				md.ATime = *object.LastModified
				md.CTime = *object.LastModified
			}

			results = append(results, md)
		}

		// Check if there are more results
		if result.NextContinuationToken == nil {
			break
		}
		input.ContinuationToken = result.NextContinuationToken
	}

	return results, nil
}

// CreateDirectory creates a directory (S3 doesn't have true directories, so we create a marker)
func (a *S3Adapter) CreateDirectory(ctx context.Context, path string) error {
	// In S3, directories are implicit. We can create a marker object if needed.
	key := a.pathToKey(path)
	if !strings.HasSuffix(key, "/") {
		key += "/"
	}

	_, err := a.client.PutObjectWithContext(ctx, &s3.PutObjectInput{
		Bucket: aws.String(a.bucketName),
		Key:    aws.String(key),
		Body:   bytes.NewReader([]byte{}), // Empty object as directory marker
	})

	if err != nil {
		return fmt.Errorf("failed to create directory marker in S3: %w", err)
	}

	a.logger.Debug("Directory created in S3",
		zap.String("bucket", a.bucketName),
		zap.String("key", key))

	return nil
}
