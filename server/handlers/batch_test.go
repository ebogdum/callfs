package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/ebogdum/callfs/auth"
	"github.com/ebogdum/callfs/core"
	"github.com/ebogdum/callfs/jobs"
	"github.com/ebogdum/callfs/metadata"
	"github.com/ebogdum/callfs/telemetry"
)

func TestBatchValidateRequest(t *testing.T) {
	ok := batchOp{Op: "delete", Path: "/a.txt"}
	if err := validateBatchRequest(batchRequest{Ops: []batchOp{ok}}); nil != err {
		t.Fatalf("valid sync batch rejected: %v", err)
	}
	if err := validateBatchRequest(batchRequest{Ops: []batchOp{ok}, Mode: "job"}); nil != err {
		t.Fatalf("valid job batch rejected: %v", err)
	}
	if err := validateBatchRequest(batchRequest{}); nil == err {
		t.Fatal("empty ops accepted")
	}
	if err := validateBatchRequest(batchRequest{Ops: make([]batchOp, batchMaxOps+1), Mode: "job"}); nil == err {
		t.Fatal("more than 10000 ops accepted")
	}
	if err := validateBatchRequest(batchRequest{Ops: make([]batchOp, batchSyncMaxOps+1)}); nil == err {
		t.Fatal("more than 100 sync ops accepted")
	}
	if err := validateBatchRequest(batchRequest{Ops: []batchOp{ok}, Mode: "async"}); nil == err {
		t.Fatal("unknown mode accepted")
	}
	if err := validateBatchRequest(batchRequest{Ops: []batchOp{{Op: "copy", Path: "/a"}}}); nil == err {
		t.Fatal("unknown op accepted")
	}
	invalid := []batchOp{
		{Op: "delete"},
		{Op: "move", Path: "/a"},
		{Op: "rename", Path: "/a"},
		{Op: "mode", Path: "/a", Mode: "0888"},
		{Op: "restore"},
	}
	for _, op := range invalid {
		if err := validateBatchOp(op); nil == err {
			t.Fatalf("validateBatchOp(%+v) accepted", op)
		}
	}
	valid := []batchOp{
		{Op: "move", Path: "/a", Destination: "/b"},
		{Op: "rename", Path: "/a", Name: "b"},
		{Op: "mode", Path: "/a", Mode: "0640"},
		{Op: "restore", Path: "trash-id"},
	}
	for _, op := range valid {
		if err := validateBatchOp(op); nil != err {
			t.Fatalf("validateBatchOp(%+v) rejected: %v", op, err)
		}
	}
}

func TestBatchPath(t *testing.T) {
	if got, err := batchPath("docs/a.txt"); nil != err || "/docs/a.txt" != got {
		t.Fatalf("batchPath relative = %q, %v", got, err)
	}
	for _, raw := range []string{"/", "/../etc"} {
		if _, err := batchPath(raw); nil == err {
			t.Fatalf("batchPath(%q) accepted", raw)
		}
	}
}

func TestBatchErrorFields(t *testing.T) {
	cases := []struct {
		name string
		err  error
		code string
	}{
		{"invalid", fmt.Errorf("%w: bad", errBatchInvalid), "BAD_REQUEST"},
		{"trash invalid", fmt.Errorf("%w: root", core.ErrTrashInvalid), "BAD_REQUEST"},
		{"invalid rename", &core.ErrInvalidRename{}, "BAD_REQUEST"},
		{"forbidden", fmt.Errorf("%w: mode", errBatchForbidden), "PERMISSION_DENIED"},
		{"permission", auth.ErrPermissionDenied, "PERMISSION_DENIED"},
		{"missing", fmt.Errorf("get: %w", metadata.ErrNotFound), "FILE_NOT_FOUND"},
		{"exists", metadata.ErrAlreadyExists, "FILE_ALREADY_EXISTS"},
		{"not empty", core.ErrDirectoryNotEmpty, "CONFLICT"},
		{"trash conflict", fmt.Errorf("%w: /a", core.ErrTrashConflict), "CONFLICT"},
		{"internal", errors.New("disk on fire"), "INTERNAL_ERROR"},
	}
	for _, c := range cases {
		if code, _ := batchErrorFields(c.err); c.code != code {
			t.Fatalf("%s: code = %s, want %s", c.name, code, c.code)
		}
	}
}

func TestBatchItemsForRetry(t *testing.T) {
	items := []batchItem{
		{index: 0, op: batchOp{Op: "delete", Path: "/a"}},
		{index: 1, op: batchOp{Op: "delete", Path: "/b"}},
		{index: 2, op: batchOp{Op: "trash", Path: "/a"}},
	}
	selected := batchItemsForRetry(items, []jobs.FailedItem{{Path: "/a"}})
	if 2 != len(selected) || 0 != selected[0].index || 2 != selected[1].index {
		t.Fatalf("retry selection = %+v", selected)
	}
}

func TestJobsParseLimit(t *testing.T) {
	if got, err := parseJobsLimit(""); nil != err || jobsDefaultLimit != got {
		t.Fatalf("default limit = %d, %v", got, err)
	}
	if got, err := parseJobsLimit("200"); nil != err || 200 != got {
		t.Fatalf("limit 200 = %d, %v", got, err)
	}
	for _, raw := range []string{"0", "-1", "201", "many"} {
		if _, err := parseJobsLimit(raw); nil == err {
			t.Fatalf("parseJobsLimit(%q) accepted", raw)
		}
	}
}

func TestJobsVisibleTo(t *testing.T) {
	job := jobs.Job{ID: "j", CreatedBy: "alice"}
	admins := auth.NewAdminSet([]string{"root-admin"})
	if !jobVisibleTo(job, "alice", admins) {
		t.Fatal("owner cannot see own job")
	}
	if !jobVisibleTo(job, "root-admin", admins) {
		t.Fatal("admin cannot see job")
	}
	if jobVisibleTo(job, "bob", admins) {
		t.Fatal("other member can see job")
	}
	if code := jobErrorStatus(jobs.ErrNotFound); http.StatusNotFound != code {
		t.Fatalf("not found status = %d", code)
	}
	if code := jobErrorStatus(jobs.ErrNothingToRetry); http.StatusConflict != code {
		t.Fatalf("nothing to retry status = %d", code)
	}
}

func TestMyEventsParseSince(t *testing.T) {
	if got, err := parseEventsSince(""); nil != err || !got.IsZero() {
		t.Fatalf("empty since = %v, %v", got, err)
	}
	want := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	if got, err := parseEventsSince("2026-10-01T12:00:00Z"); nil != err || !got.Equal(want) {
		t.Fatalf("since = %v, %v", got, err)
	}
	if _, err := parseEventsSince("yesterday"); nil == err {
		t.Fatal("malformed since accepted")
	}
	if got, err := parseEventsLimit(""); nil != err || 0 != got {
		t.Fatalf("empty events limit = %d, %v", got, err)
	}
	if _, err := parseEventsLimit(fmt.Sprint(telemetry.MaxPageLimit + 1)); nil == err {
		t.Fatal("events limit above max accepted")
	}
}
