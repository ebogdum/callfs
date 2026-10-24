package internalproxy

import (
	"context"
	"errors"
	"io"

	"github.com/ebogdum/callfs/v2/metadata"
)

type Control interface {
	Stat(ctx context.Context, instanceID, path string) (*metadata.Metadata, error)
	List(ctx context.Context, instanceID, path string) ([]*metadata.Metadata, error)
	Delete(ctx context.Context, instanceID, path string, recursive bool) error
	Move(ctx context.Context, instanceID, srcPath string, body []byte) error
	Mkdir(ctx context.Context, instanceID, path string) error
}

type ByteTransport interface {
	Open(ctx context.Context, instanceID, path string) (io.ReadCloser, error)
	OpenRange(ctx context.Context, instanceID, path string, offset, length int64) (io.ReadCloser, error)
	Create(ctx context.Context, instanceID, path string, reader io.Reader, size int64, ifMatch string, ifNoneMatch bool) (string, error)
	Update(ctx context.Context, instanceID, path string, reader io.Reader, size int64, ifMatch string, ifNoneMatch bool) (string, error)
}

type Option func(*InternalProxyAdapter)

func WithControl(control Control) Option {
	return func(a *InternalProxyAdapter) {
		a.control = control
	}
}

func WithByteTransport(bytes ByteTransport) Option {
	return func(a *InternalProxyAdapter) {
		a.bytes = bytes
	}
}

func NewBadRequestError(message string) *BadRequestError {
	return &BadRequestError{message: message}
}

func wrapMoveError(err error) error {
	if nil == err {
		return nil
	}
	var badRequest *BadRequestError
	if errors.Is(err, metadata.ErrNotFound) || errors.Is(err, metadata.ErrAlreadyExists) || errors.As(err, &badRequest) {
		return err
	}
	return &ProxyError{err: err}
}
