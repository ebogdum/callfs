package telemetry

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestTransferTrackerLifecycle(t *testing.T) {
	tracker := NewTransferTracker()
	handle := tracker.Begin("download", "alice", "/a/b.bin", "192.0.2.1", 100)
	handle.Add(40)

	items := tracker.Snapshot()
	if 1 != len(items) {
		t.Fatalf("got %d transfers, want 1", len(items))
	}
	item := items[0]
	if "alice" != item.UserID || "download" != item.Kind || "/a/b.bin" != item.Path || "192.0.2.1" != item.Client {
		t.Fatalf("unexpected transfer: %+v", item)
	}
	if 40 != item.BytesDone || nil == item.BytesTotal || 100 != *item.BytesTotal {
		t.Fatalf("unexpected progress: done=%d total=%v", item.BytesDone, item.BytesTotal)
	}
	if "" == item.ID || item.StartedAt.IsZero() {
		t.Fatalf("missing id or start: %+v", item)
	}

	handle.Finish()
	handle.Finish()
	if 0 != len(tracker.Snapshot()) {
		t.Fatalf("transfer still listed after finish")
	}
}

func TestTransferTrackerUnknownTotal(t *testing.T) {
	tracker := NewTransferTracker()
	handle := tracker.Begin("upload", "bob", "/c", "", -1)
	defer handle.Finish()
	if nil != tracker.Snapshot()[0].BytesTotal {
		t.Fatalf("unknown total must be null")
	}
	handle.SetTotal(7)
	if total := tracker.Snapshot()[0].BytesTotal; nil == total || 7 != *total {
		t.Fatalf("total not updated: %v", total)
	}
}

func TestBeginIfLargeSkipsSmallTransfers(t *testing.T) {
	tracker := NewTransferTracker()
	if nil != tracker.BeginIfLarge("download", "u", "/small", "", MinTrackedTransferBytes-1) {
		t.Fatalf("small transfer must not be tracked")
	}
	large := tracker.BeginIfLarge("download", "u", "/large", "", MinTrackedTransferBytes)
	if nil == large || 1 != len(tracker.Snapshot()) {
		t.Fatalf("large transfer must be tracked")
	}
	large.Finish()
	chunked := tracker.BeginIfLarge("upload", "u", "/chunked", "", -1)
	if nil == chunked {
		t.Fatalf("unknown size must be tracked")
	}
	chunked.Finish()

	var none *TransferHandle
	none.Add(1)
	none.SetTotal(1)
	none.Finish()
}

func TestTransferHandleCountsBodyAndWriter(t *testing.T) {
	tracker := NewTransferTracker()
	handle := tracker.Begin("upload", "u", "/p", "", 11)
	defer handle.Finish()

	body := handle.Body(io.NopCloser(strings.NewReader("hello world")))
	if _, err := io.Copy(io.Discard, body); nil != err {
		t.Fatalf("read body: %v", err)
	}
	if 11 != tracker.Snapshot()[0].BytesDone {
		t.Fatalf("body bytes = %d, want 11", tracker.Snapshot()[0].BytesDone)
	}

	var sink bytes.Buffer
	if _, err := io.Copy(handle.Writer(&sink), strings.NewReader("abc")); nil != err {
		t.Fatalf("write: %v", err)
	}
	if 14 != tracker.Snapshot()[0].BytesDone || "abc" != sink.String() {
		t.Fatalf("writer bytes = %d sink=%q", tracker.Snapshot()[0].BytesDone, sink.String())
	}
}
