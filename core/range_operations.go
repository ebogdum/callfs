package core

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/ebogdum/callfs/v2/metadata"
)

func (e *Engine) GetFileRange(ctx context.Context, path string, offset, length int64) (io.ReadCloser, *metadata.Metadata, error) {
	md, err := e.GetMetadata(ctx, path)
	if nil != err {
		return nil, nil, fmt.Errorf("failed to get file metadata: %w", err)
	}

	reader, err := e.OpenFileRange(ctx, md, path, offset, length)
	if nil != err {
		return nil, nil, err
	}
	return reader, md, nil
}

func (e *Engine) OpenFileRange(ctx context.Context, md *metadata.Metadata, path string, offset, length int64) (io.ReadCloser, error) {
	if "file" != md.Type {
		return nil, fmt.Errorf("path is not a file")
	}

	if em := e.GetErasureManager(); md.ErasureCoded && em != nil {
		reader, err := em.OpenRangeReader(ctx, path, offset, length)
		if nil != err {
			return nil, fmt.Errorf("failed to retrieve erasure-coded file range: %w", err)
		}
		return reader, nil
	}

	ctx, storage := e.selectBackend(ctx, md)

	relativePath := strings.TrimPrefix(path, "/")
	reader, err := storage.OpenRange(ctx, relativePath, offset, length)
	if nil != err {
		return nil, fmt.Errorf("failed to open file range: %w", err)
	}

	return reader, nil
}
