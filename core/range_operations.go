package core

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/ebogdum/callfs/v2/metadata"
)

func (e *Engine) OpenFileRange(ctx context.Context, md *metadata.Metadata, path string, offset, length int64) (io.ReadCloser, error) {
	if "file" != md.Type {
		return nil, fmt.Errorf("path is not a file")
	}

	if em := e.GetErasureManager(); md.ErasureCoded && em != nil {
		reader, err := em.OpenRangeReader(ctx, path, offset, length)
		if nil == err {
			return reader, nil
		}
		if nil != ctx.Err() {
			return nil, fmt.Errorf("failed to retrieve erasure-coded file range: %w", err)
		}
		data, rangeErr := em.RetrieveRange(ctx, path, offset, length)
		if nil != rangeErr {
			return nil, fmt.Errorf("failed to retrieve erasure-coded file range: %w", errors.Join(err, rangeErr))
		}
		return io.NopCloser(bytes.NewReader(data)), nil
	}

	ctx, storage := e.selectBackend(ctx, md)

	relativePath := strings.TrimPrefix(path, "/")
	reader, err := storage.OpenRange(ctx, relativePath, offset, length)
	if nil != err {
		return nil, fmt.Errorf("failed to open file range: %w", err)
	}

	return reader, nil
}
