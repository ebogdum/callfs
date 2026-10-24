package core

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/v2/auth"
	"github.com/ebogdum/callfs/v2/erasure"
	"github.com/ebogdum/callfs/v2/metadata"
)

type fakeUploadEngine struct {
	files map[string][]byte
}

func (f *fakeUploadEngine) GetMetadata(_ context.Context, path string) (*metadata.Metadata, error) {
	data, ok := f.files[path]
	if !ok {
		return nil, metadata.ErrNotFound
	}
	return &metadata.Metadata{Path: path, Name: path, Type: "file", Size: int64(len(data))}, nil
}

func (f *fakeUploadEngine) PutFile(_ context.Context, path string, reader io.Reader, _ int64, md *metadata.Metadata, _ string, ifNoneMatch bool) (*metadata.Metadata, bool, error) {
	_, exists := f.files[path]
	if ifNoneMatch && exists {
		return nil, false, metadata.ErrPrecondition
	}
	data, err := io.ReadAll(reader)
	if nil != err {
		return nil, false, err
	}
	f.files[path] = data
	return &metadata.Metadata{Path: path, Name: md.Name, Type: "file", Size: int64(len(data)), ETag: "etag-1"}, !exists, nil
}

func (f *fakeUploadEngine) CreateErasureFile(_ context.Context, _ string, _ io.Reader, _ *erasure.StoreOptions, _ func(*erasure.ErasureFileInfo) *metadata.Metadata) (*metadata.Metadata, error) {
	return nil, errors.New("erasure is not expected in this test")
}

type uploadTestAuthorizer struct{}

func (uploadTestAuthorizer) Authorize(context.Context, string, string, auth.PermissionType) error {
	return nil
}

func (uploadTestAuthorizer) AuthorizeWithMetadata(context.Context, string, string, *metadata.Metadata, auth.PermissionType) error {
	return nil
}

func newTestUploads(t *testing.T, engine UploadEngine) *Uploads {
	t.Helper()
	uploads, err := NewUploads(engine, uploadTestAuthorizer{}, t.TempDir(), "localfs", zap.NewNop())
	if nil != err {
		t.Fatalf("new uploads: %v", err)
	}
	return uploads
}

func TestUploadsCompleteAssemblesParts(t *testing.T) {
	engine := &fakeUploadEngine{files: map[string][]byte{}}
	uploads := newTestUploads(t, engine)
	payload := []byte("0123456789")

	session, err := uploads.Create(context.Background(), "alice", UploadRequest{Path: "/docs/a.txt", Size: int64(len(payload))})
	if nil != err {
		t.Fatalf("create upload: %v", err)
	}
	if _, err := uploads.PutPart("alice", session.ID, UploadPart{Start: 0, End: 4, Total: 10}, bytes.NewReader(payload[:5])); nil != err {
		t.Fatalf("put first part: %v", err)
	}
	if _, err := uploads.PutPart("alice", session.ID, UploadPart{Start: 5, End: 9, Total: 10}, bytes.NewReader(payload[5:])); nil != err {
		t.Fatalf("put second part: %v", err)
	}

	result, err := uploads.Complete(context.Background(), "alice", session.ID)
	if nil != err {
		t.Fatalf("complete upload: %v", err)
	}
	if !result.Created || "/docs/a.txt" != result.Path || "etag-1" != result.ETag {
		t.Fatalf("unexpected result %+v", result)
	}
	if !bytes.Equal(engine.files["/docs/a.txt"], payload) {
		t.Fatalf("assembled content %q, want %q", engine.files["/docs/a.txt"], payload)
	}
}

func TestUploadsPutPartRejectsOffsetMismatch(t *testing.T) {
	uploads := newTestUploads(t, &fakeUploadEngine{files: map[string][]byte{}})
	session, err := uploads.Create(context.Background(), "alice", UploadRequest{Path: "/docs/a.txt", Size: 10})
	if nil != err {
		t.Fatalf("create upload: %v", err)
	}

	_, err = uploads.PutPart("alice", session.ID, UploadPart{Start: 3, End: 9, Total: 10}, bytes.NewReader(make([]byte, 7)))
	var offsetErr *UploadOffsetError
	if !errors.As(err, &offsetErr) || 0 != offsetErr.Offset {
		t.Fatalf("expected offset mismatch at 0, got %v", err)
	}
}

func TestUploadsCompleteRequiresAllBytes(t *testing.T) {
	uploads := newTestUploads(t, &fakeUploadEngine{files: map[string][]byte{}})
	session, err := uploads.Create(context.Background(), "alice", UploadRequest{Path: "/docs/a.txt", Size: 10})
	if nil != err {
		t.Fatalf("create upload: %v", err)
	}
	if _, err := uploads.PutPart("alice", session.ID, UploadPart{Start: 0, End: 4, Total: 10}, bytes.NewReader(make([]byte, 5))); nil != err {
		t.Fatalf("put part: %v", err)
	}

	if _, err := uploads.Complete(context.Background(), "alice", session.ID); !errors.Is(err, ErrUploadIncomplete) {
		t.Fatalf("expected ErrUploadIncomplete, got %v", err)
	}
}

func TestUploadsHideSessionsFromOtherUsers(t *testing.T) {
	uploads := newTestUploads(t, &fakeUploadEngine{files: map[string][]byte{}})
	session, err := uploads.Create(context.Background(), "alice", UploadRequest{Path: "/docs/a.txt", Size: 0})
	if nil != err {
		t.Fatalf("create upload: %v", err)
	}

	if _, err := uploads.Status("bob", session.ID); !errors.Is(err, ErrUploadNotFound) {
		t.Fatalf("expected ErrUploadNotFound for other user, got %v", err)
	}
}

func TestUploadsCreateRejectsExistingFileWithoutOverwrite(t *testing.T) {
	engine := &fakeUploadEngine{files: map[string][]byte{"/docs/a.txt": []byte("x")}}
	uploads := newTestUploads(t, engine)

	if _, err := uploads.Create(context.Background(), "alice", UploadRequest{Path: "/docs/a.txt", Size: 1}); !errors.Is(err, ErrUploadExists) {
		t.Fatalf("expected ErrUploadExists, got %v", err)
	}
}

func TestUploadsCleanupRemovesExpiredSessions(t *testing.T) {
	uploads := newTestUploads(t, &fakeUploadEngine{files: map[string][]byte{}})
	session, err := uploads.Create(context.Background(), "alice", UploadRequest{Path: "/docs/a.txt", Size: 0})
	if nil != err {
		t.Fatalf("create upload: %v", err)
	}

	if removed, err := uploads.Cleanup(time.Now()); nil != err || 0 != removed {
		t.Fatalf("cleanup before expiry removed %d, err %v", removed, err)
	}
	removed, err := uploads.Cleanup(time.Now().Add(UploadSessionTTL + time.Hour))
	if nil != err || 1 != removed {
		t.Fatalf("cleanup after expiry removed %d, err %v", removed, err)
	}
	if _, err := uploads.Status("alice", session.ID); !errors.Is(err, ErrUploadNotFound) {
		t.Fatalf("expected removed session to be missing, got %v", err)
	}
}
