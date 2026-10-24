package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/v2/auth"
	"github.com/ebogdum/callfs/v2/core"
	"github.com/ebogdum/callfs/v2/metadata"
)

func TestParseTrashLimit(t *testing.T) {
	if got, err := parseTrashLimit(""); nil != err || trashListDefaultLimit != got {
		t.Fatalf("default limit = %d, %v", got, err)
	}
	if got, err := parseTrashLimit("1000"); nil != err || 1000 != got {
		t.Fatalf("limit 1000 = %d, %v", got, err)
	}
	for _, raw := range []string{"0", "-3", "1001", "many"} {
		if _, err := parseTrashLimit(raw); nil == err {
			t.Fatalf("parseTrashLimit(%q) accepted", raw)
		}
	}
}

func TestDecodeTrashRestoreBody(t *testing.T) {
	empty, err := decodeTrashRestoreBody(strings.NewReader(""))
	if nil != err || "" != empty.To || "" != empty.OnConflict {
		t.Fatalf("empty body = %+v, %v", empty, err)
	}
	body, err := decodeTrashRestoreBody(strings.NewReader(`{"to":"/docs/a.txt","on_conflict":"keep_both"}`))
	if nil != err || "/docs/a.txt" != body.To || "keep_both" != body.OnConflict {
		t.Fatalf("body = %+v, %v", body, err)
	}
	if _, err := decodeTrashRestoreBody(strings.NewReader(`{"to":`)); nil == err {
		t.Fatal("malformed body accepted")
	}
}

func TestSendTrashErrorStatus(t *testing.T) {
	cases := []struct {
		name string
		err  error
		code int
		want string
	}{
		{"invalid", fmt.Errorf("%w: bad", core.ErrTrashInvalid), http.StatusBadRequest, "BAD_REQUEST"},
		{"conflict", fmt.Errorf("%w: /a.txt", core.ErrTrashConflict), http.StatusConflict, "CONFLICT"},
		{"forbidden", fmt.Errorf("%w: other user", auth.ErrPermissionDenied), http.StatusForbidden, "PERMISSION_DENIED"},
		{"missing", fmt.Errorf("trash record: %w", metadata.ErrNotFound), http.StatusNotFound, "FILE_NOT_FOUND"},
		{"internal", errors.New("disk on fire"), http.StatusInternalServerError, "INTERNAL_ERROR"},
	}
	for _, c := range cases {
		recorder := httptest.NewRecorder()
		sendTrashError(recorder, zap.NewNop(), c.err)
		if c.code != recorder.Code {
			t.Fatalf("%s: status = %d, want %d", c.name, recorder.Code, c.code)
		}
		var response ErrorResponse
		if err := json.Unmarshal(recorder.Body.Bytes(), &response); nil != err {
			t.Fatalf("%s: decode body: %v", c.name, err)
		}
		if c.want != response.Code {
			t.Fatalf("%s: code = %q, want %q", c.name, response.Code, c.want)
		}
	}
}
