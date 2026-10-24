package backends

import (
	"context"
	"io"
	"time"

	"github.com/ebogdum/callfs/v2/metadata"
	"github.com/ebogdum/callfs/v2/metrics"
)

type instrumentedStorage struct {
	Storage
	backendType string
}

func Instrument(name string, s Storage) Storage {
	if nil == s {
		return nil
	}
	return &instrumentedStorage{Storage: s, backendType: name}
}

func (s *instrumentedStorage) Unwrap() Storage {
	return s.Storage
}

func (s *instrumentedStorage) observe(operation string, start time.Time) {
	metrics.BackendOpsTotal.WithLabelValues(s.backendType, operation).Inc()
	metrics.BackendOpDuration.WithLabelValues(s.backendType, operation).Observe(time.Since(start).Seconds())
}

func (s *instrumentedStorage) Open(ctx context.Context, path string) (io.ReadCloser, error) {
	defer s.observe("open", time.Now())
	return s.Storage.Open(ctx, path)
}

func (s *instrumentedStorage) OpenRange(ctx context.Context, path string, offset, length int64) (io.ReadCloser, error) {
	defer s.observe("open_range", time.Now())
	return s.Storage.OpenRange(ctx, path, offset, length)
}

func (s *instrumentedStorage) Create(ctx context.Context, path string, reader io.Reader, size int64) error {
	defer s.observe("create", time.Now())
	return s.Storage.Create(ctx, path, reader, size)
}

func (s *instrumentedStorage) Update(ctx context.Context, path string, reader io.Reader, size int64) error {
	defer s.observe("update", time.Now())
	return s.Storage.Update(ctx, path, reader, size)
}

func (s *instrumentedStorage) Delete(ctx context.Context, path string) error {
	defer s.observe("delete", time.Now())
	return s.Storage.Delete(ctx, path)
}

func (s *instrumentedStorage) Move(ctx context.Context, oldPath, newPath string) error {
	defer s.observe("move", time.Now())
	return s.Storage.Move(ctx, oldPath, newPath)
}

func (s *instrumentedStorage) Stat(ctx context.Context, path string) (*metadata.Metadata, error) {
	defer s.observe("stat", time.Now())
	return s.Storage.Stat(ctx, path)
}

func (s *instrumentedStorage) ListDirectory(ctx context.Context, path string) ([]*metadata.Metadata, error) {
	defer s.observe("list_directory", time.Now())
	return s.Storage.ListDirectory(ctx, path)
}

func (s *instrumentedStorage) CreateDirectory(ctx context.Context, path string) error {
	defer s.observe("create_directory", time.Now())
	return s.Storage.CreateDirectory(ctx, path)
}

func (s *instrumentedStorage) Close() error {
	defer s.observe("close", time.Now())
	return s.Storage.Close()
}
