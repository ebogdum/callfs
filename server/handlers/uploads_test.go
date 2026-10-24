package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/auth"
	"github.com/ebogdum/callfs/core"
	"github.com/ebogdum/callfs/erasure"
	"github.com/ebogdum/callfs/metadata"
)

type noUploadEngine struct{}

func (noUploadEngine) GetMetadata(context.Context, string) (*metadata.Metadata, error) {
	return nil, metadata.ErrNotFound
}

func (noUploadEngine) PutFile(context.Context, string, io.Reader, int64, *metadata.Metadata, string, bool) (*metadata.Metadata, bool, error) {
	return nil, false, errors.New("unused")
}

func (noUploadEngine) CreateErasureFile(context.Context, string, io.Reader, *erasure.StoreOptions, func(*erasure.ErasureFileInfo) *metadata.Metadata) (*metadata.Metadata, error) {
	return nil, errors.New("unused")
}

type allowAllUploadAuthorizer struct{}

func (allowAllUploadAuthorizer) Authorize(context.Context, string, string, auth.PermissionType) error {
	return nil
}

func (allowAllUploadAuthorizer) AuthorizeWithMetadata(context.Context, string, string, *metadata.Metadata, auth.PermissionType) error {
	return nil
}

func newUploadTestUploads(t *testing.T) *core.Uploads {
	t.Helper()
	uploads, err := core.NewUploads(noUploadEngine{}, allowAllUploadAuthorizer{}, t.TempDir(), "localfs", zap.NewNop())
	if nil != err {
		t.Fatalf("new uploads: %v", err)
	}
	return uploads
}

func TestSendUploadErrorMapsStatusCodes(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		code   string
		offset bool
	}{
		{name: "offset mismatch", err: &core.UploadOffsetError{Offset: 4}, status: http.StatusConflict, code: "CONFLICT", offset: true},
		{name: "not found", err: core.ErrUploadNotFound, status: http.StatusNotFound, code: "NOT_FOUND"},
		{name: "too large", err: core.ErrUploadTooLarge, status: http.StatusRequestEntityTooLarge, code: "PAYLOAD_TOO_LARGE"},
		{name: "precondition", err: metadata.ErrPrecondition, status: http.StatusPreconditionFailed, code: "PRECONDITION_FAILED"},
		{name: "invalid", err: core.ErrUploadRange, status: http.StatusBadRequest, code: "BAD_REQUEST"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			sendUploadError(recorder, zap.NewNop(), tc.err)
			if tc.status != recorder.Code {
				t.Fatalf("status %d, want %d", recorder.Code, tc.status)
			}
			var body uploadErrorBody
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); nil != err {
				t.Fatalf("decode body: %v", err)
			}
			if tc.code != body.Code {
				t.Fatalf("code %q, want %q", body.Code, tc.code)
			}
			if tc.offset {
				if offset, ok := body.Details["offset"].(float64); !ok || 4 != offset {
					t.Fatalf("details %v, want offset 4", body.Details)
				}
			}
		})
	}
}

func TestParseUploadContentRange(t *testing.T) {
	part, err := parseUploadContentRange("bytes 0-7/16")
	if nil != err || 0 != part.Start || 7 != part.End || 16 != part.Total {
		t.Fatalf("parsed %+v, err %v", part, err)
	}
	for _, raw := range []string{"", "bytes 0-7", "bytes 9-x/16", "items 0-7/16"} {
		if _, err := parseUploadContentRange(raw); !errors.Is(err, core.ErrUploadRange) {
			t.Fatalf("Content-Range %q: expected ErrUploadRange, got %v", raw, err)
		}
	}
}

func TestV1GetUploadRequiresAuthentication(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/uploads/00000000000000000000000000000000", nil)
	V1GetUpload(newUploadTestUploads(t))(recorder, request)
	if http.StatusUnauthorized != recorder.Code {
		t.Fatalf("status %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}
