package s3

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"go.uber.org/zap"

	"github.com/ebogdum/callfs/metadata"
)

const maxDirectoryListEntries = 100000

// ListDirectory lists the contents of a directory
func (a *S3Adapter) ListDirectory(ctx context.Context, path string) ([]*metadata.Metadata, error) {
	var results []*metadata.Metadata
	err := a.ListDirectoryEach(ctx, path, func(md *metadata.Metadata) error {
		if maxDirectoryListEntries <= len(results) {
			return fmt.Errorf("directory listing exceeds %d entries", maxDirectoryListEntries)
		}
		results = append(results, md)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return results, nil
}

func (a *S3Adapter) ListDirectoryEach(ctx context.Context, path string, fn func(*metadata.Metadata) error) error {
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

	now := time.Now()

	for {
		result, err := a.client.ListObjectsV2(ctx, input)
		if err != nil {
			return fmt.Errorf("failed to list objects in S3: %w", err)
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
				BackendType: "s3",
				ATime:       now,
				MTime:       now,
				CTime:       now,
			}

			if err := fn(md); err != nil {
				return err
			}
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
				Size:        aws.ToInt64(object.Size),
				Mode:        defaultFileMode,
				BackendType: "s3",
			}

			if object.LastModified != nil {
				md.MTime = *object.LastModified
				md.ATime = *object.LastModified
				md.CTime = *object.LastModified
			}

			if err := fn(md); err != nil {
				return err
			}
		}

		// Check if there are more results
		if result.NextContinuationToken == nil {
			return nil
		}
		input.ContinuationToken = result.NextContinuationToken
	}
}

// CreateDirectory creates a directory (S3 doesn't have true directories, so we create a marker)
func (a *S3Adapter) CreateDirectory(ctx context.Context, path string) error {
	// In S3, directories are implicit. We can create a marker object if needed.
	key := a.pathToKey(path)
	if !strings.HasSuffix(key, "/") {
		key += "/"
	}

	_, err := a.client.PutObject(ctx, &s3.PutObjectInput{
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
