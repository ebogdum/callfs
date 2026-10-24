package s3

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"go.uber.org/zap"

	"github.com/ebogdum/callfs/metadata"
)

const (
	modeMetadataKey          = "callfs-mode"
	modeMetadataKeyCanonical = "Callfs-Mode"
	defaultFileMode          = "0644"
	copyObjectMaxSize        = 5 * 1024 * 1024 * 1024
	copyPartSize             = 1024 * 1024 * 1024
)

type modeContextKey struct{}

var (
	errBodyExceedsDeclaredSize = errors.New("request body exceeds declared size")
	errBodyShorterThanDeclared = errors.New("request body shorter than declared size")
)

type exactSizeReader struct {
	r         io.Reader
	remaining int64
}

func (e *exactSizeReader) Read(p []byte) (int, error) {
	if int64(len(p)) > e.remaining+1 {
		p = p[:e.remaining+1]
	}
	n, err := e.r.Read(p)
	e.remaining -= int64(n)
	if 0 > e.remaining {
		return 0, errBodyExceedsDeclaredSize
	}
	if io.EOF == err && 0 < e.remaining {
		return n, errBodyShorterThanDeclared
	}
	return n, err
}

var uploadBufferPool = sync.Pool{
	New: func() any {
		buf := make([]byte, uploadPartSize)
		return &buf
	},
}

func (a *S3Adapter) Open(ctx context.Context, path string) (io.ReadCloser, error) {
	key := a.pathToKey(path)

	result, err := a.client.GetObject(ctx, &s3.GetObjectInput{
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

func (a *S3Adapter) OpenRange(ctx context.Context, path string, offset, length int64) (io.ReadCloser, error) {
	if offset < 0 || length < -1 || length == 0 {
		return nil, fmt.Errorf("invalid range offset=%d length=%d", offset, length)
	}

	key := a.pathToKey(path)

	rangeHeader := fmt.Sprintf("bytes=%d-", offset)
	if length > 0 {
		rangeHeader = fmt.Sprintf("bytes=%d-%d", offset, offset+length-1)
	}

	result, err := a.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(a.bucketName),
		Key:    aws.String(key),
		Range:  aws.String(rangeHeader),
	})

	if err != nil {
		if isS3NotFound(err) {
			return nil, metadata.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get object range from S3: %w", err)
	}

	a.logger.Debug("File range opened from S3",
		zap.String("bucket", a.bucketName),
		zap.String("key", key),
		zap.Int64("offset", offset),
		zap.Int64("length", length))

	return result.Body, nil
}

func (a *S3Adapter) Create(ctx context.Context, path string, reader io.Reader, size int64) error {
	key := a.pathToKey(path)

	body := reader
	if 0 <= size {
		body = &exactSizeReader{r: reader, remaining: size}
	}

	putInput := &s3.PutObjectInput{
		Bucket: aws.String(a.bucketName),
		Key:    aws.String(key),
		Body:   body,
	}

	if mode, ok := modeFromContext(ctx); ok {
		putInput.Metadata = map[string]string{modeMetadataKey: mode}
	}

	if a.serverSideEncryption != "" {
		putInput.ServerSideEncryption = types.ServerSideEncryption(a.serverSideEncryption)
		if a.serverSideEncryption == "aws:kms" && a.kmsKeyID != "" {
			putInput.SSEKMSKeyId = aws.String(a.kmsKeyID)
		}
	}

	if a.acl != "" {
		putInput.ACL = types.ObjectCannedACL(a.acl)
	}

	if contentType := getContentType(path); contentType != "" {
		putInput.ContentType = aws.String(contentType)
	}

	if err := a.upload(ctx, putInput, size); err != nil {
		return fmt.Errorf("failed to put object to S3: %w", err)
	}

	a.logger.Debug("File created in S3",
		zap.String("bucket", a.bucketName),
		zap.String("key", key),
		zap.Int64("size", size))

	return nil
}

func (a *S3Adapter) upload(ctx context.Context, input *s3.PutObjectInput, size int64) error {
	if size < 0 || size >= uploadPartSize {
		_, err := a.uploader.Upload(ctx, input)
		return err
	}

	buf := uploadBufferPool.Get().(*[]byte)
	defer uploadBufferPool.Put(buf)

	n, err := io.ReadFull(input.Body, *buf)
	switch {
	case err == nil:
		input.Body = io.MultiReader(bytes.NewReader((*buf)[:n]), input.Body)
		_, err = a.uploader.Upload(ctx, input)
		return err
	case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
	default:
		return err
	}

	_, err = a.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:               input.Bucket,
		Key:                  input.Key,
		Body:                 bytes.NewReader((*buf)[:n]),
		Metadata:             input.Metadata,
		ServerSideEncryption: input.ServerSideEncryption,
		SSEKMSKeyId:          input.SSEKMSKeyId,
		ACL:                  input.ACL,
		ContentType:          input.ContentType,
	})
	return err
}

func (a *S3Adapter) Update(ctx context.Context, path string, reader io.Reader, size int64) error {
	return a.Create(ctx, path, reader, size)
}

func (a *S3Adapter) Delete(ctx context.Context, path string) error {
	key := a.pathToKey(path)

	_, err := a.client.DeleteObject(ctx, &s3.DeleteObjectInput{
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

func (a *S3Adapter) Move(ctx context.Context, oldPath, newPath string) error {
	srcKey := a.pathToKey(oldPath)
	dstKey := a.pathToKey(newPath)
	if srcKey == dstKey {
		return nil
	}

	escapedSegments := strings.Split(srcKey, "/")
	for i, segment := range escapedSegments {
		escapedSegments[i] = url.PathEscape(segment)
	}

	copyInput := &s3.CopyObjectInput{
		Bucket:     aws.String(a.bucketName),
		CopySource: aws.String(a.bucketName + "/" + strings.Join(escapedSegments, "/")),
		Key:        aws.String(dstKey),
	}
	if a.serverSideEncryption != "" {
		copyInput.ServerSideEncryption = types.ServerSideEncryption(a.serverSideEncryption)
		if a.serverSideEncryption == "aws:kms" && a.kmsKeyID != "" {
			copyInput.SSEKMSKeyId = aws.String(a.kmsKeyID)
		}
	}
	if a.acl != "" {
		copyInput.ACL = types.ObjectCannedACL(a.acl)
	}

	head, err := a.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(a.bucketName),
		Key:    aws.String(srcKey),
	})
	if err != nil {
		if isS3NotFound(err) {
			return metadata.ErrNotFound
		}
		return fmt.Errorf("failed to stat object during move: %w", err)
	}

	size := aws.ToInt64(head.ContentLength)
	if copyObjectMaxSize < size {
		err = a.multipartCopy(ctx, copyInput, size, head.ContentType, head.Metadata)
	} else {
		_, err = a.client.CopyObject(ctx, copyInput)
	}
	if err != nil {
		if isS3NotFound(err) {
			return metadata.ErrNotFound
		}
		return fmt.Errorf("failed to copy object during move: %w", err)
	}

	if _, err := a.client.DeleteObject(ctx, &s3.DeleteObjectInput{
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

func (a *S3Adapter) multipartCopy(ctx context.Context, copyInput *s3.CopyObjectInput, size int64, contentType *string, metadata map[string]string) error {
	created, err := a.client.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{
		Bucket:               copyInput.Bucket,
		Key:                  copyInput.Key,
		ContentType:          contentType,
		Metadata:             metadata,
		ServerSideEncryption: copyInput.ServerSideEncryption,
		SSEKMSKeyId:          copyInput.SSEKMSKeyId,
		ACL:                  copyInput.ACL,
	})
	if err != nil {
		return fmt.Errorf("failed to create multipart copy: %w", err)
	}

	a.logger.Debug("Multipart copy started",
		zap.String("bucket", a.bucketName),
		zap.String("key", aws.ToString(copyInput.Key)),
		zap.Int64("size", size))

	var parts []types.CompletedPart
	for partNumber, offset := int32(1), int64(0); offset < size; partNumber, offset = partNumber+1, offset+copyPartSize {
		end := offset + copyPartSize - 1
		if size-1 < end {
			end = size - 1
		}
		out, err := a.client.UploadPartCopy(ctx, &s3.UploadPartCopyInput{
			Bucket:          copyInput.Bucket,
			Key:             copyInput.Key,
			UploadId:        created.UploadId,
			PartNumber:      aws.Int32(partNumber),
			CopySource:      copyInput.CopySource,
			CopySourceRange: aws.String(fmt.Sprintf("bytes=%d-%d", offset, end)),
		})
		if err != nil {
			return a.abortMultipartCopy(ctx, copyInput, created.UploadId, fmt.Errorf("failed to copy part %d: %w", partNumber, err))
		}
		if nil == out.CopyPartResult {
			return a.abortMultipartCopy(ctx, copyInput, created.UploadId, fmt.Errorf("missing result for copied part %d", partNumber))
		}
		parts = append(parts, types.CompletedPart{ETag: out.CopyPartResult.ETag, PartNumber: aws.Int32(partNumber)})
	}

	if _, err := a.client.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
		Bucket:          copyInput.Bucket,
		Key:             copyInput.Key,
		UploadId:        created.UploadId,
		MultipartUpload: &types.CompletedMultipartUpload{Parts: parts},
	}); err != nil {
		return a.abortMultipartCopy(ctx, copyInput, created.UploadId, fmt.Errorf("failed to complete multipart copy: %w", err))
	}

	return nil
}

func (a *S3Adapter) abortMultipartCopy(ctx context.Context, copyInput *s3.CopyObjectInput, uploadID *string, cause error) error {
	if _, err := a.client.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{
		Bucket:   copyInput.Bucket,
		Key:      copyInput.Key,
		UploadId: uploadID,
	}); err != nil {
		return fmt.Errorf("%w; abort of multipart copy failed: %v", cause, err)
	}
	return cause
}

func WithMode(ctx context.Context, mode string) context.Context {
	return context.WithValue(ctx, modeContextKey{}, mode)
}

func modeFromContext(ctx context.Context) (string, bool) {
	mode, ok := ctx.Value(modeContextKey{}).(string)
	return mode, ok && mode != ""
}

func (a *S3Adapter) Stat(ctx context.Context, path string) (*metadata.Metadata, error) {
	key := a.pathToKey(path)

	result, err := a.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(a.bucketName),
		Key:    aws.String(key),
	})

	if err != nil {
		if isS3NotFound(err) {
			return nil, metadata.ErrNotFound
		}
		return nil, fmt.Errorf("failed to stat object in S3: %w", err)
	}

	mode := result.Metadata[modeMetadataKeyCanonical]
	if mode == "" {
		mode = defaultFileMode
	}

	md := &metadata.Metadata{
		Name:        filepath.Base(path),
		Path:        "/" + key,
		Type:        "file",
		Size:        aws.ToInt64(result.ContentLength),
		Mode:        mode,
		BackendType: "s3",
	}

	if result.LastModified != nil {
		md.MTime = *result.LastModified
		md.ATime = *result.LastModified
		md.CTime = *result.LastModified
	}

	return md, nil
}

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
