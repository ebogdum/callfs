package handlers

import (
	"bytes"
	"io"
	"testing"
)

// writeChunks feeds data to the spool in fixed-size chunks, as the websocket
// read loop does.
func writeChunks(t *testing.T, s *uploadSpool, data []byte, chunk int) {
	t.Helper()
	for start := 0; start < len(data); start += chunk {
		end := start + chunk
		if end > len(data) {
			end = len(data)
		}
		if err := s.Write(data[start:end]); err != nil {
			t.Fatalf("spool write at offset %d: %v", start, err)
		}
	}
}

// deterministicPayload builds a byte pattern where every position is distinct
// enough that a mis-ordered or truncated spool is detected.
func deterministicPayload(n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = byte(i % 251)
	}
	return out
}

// TestUploadSpoolRoundTrip checks payloads on both sides of the spill threshold
// come back byte-identical. The spill boundary is where a bug would silently
// corrupt or truncate an upload, so it is covered explicitly.
func TestUploadSpoolRoundTrip(t *testing.T) {
	sizes := []struct {
		name string
		size int
	}{
		{"empty", 0},
		{"tiny", 1},
		{"in memory", 1024},
		{"just below threshold", wsSpillThreshold - 1},
		{"exactly threshold", wsSpillThreshold},
		{"just above threshold", wsSpillThreshold + 1},
		{"well above threshold", wsSpillThreshold * 2},
	}

	for _, tc := range sizes {
		t.Run(tc.name, func(t *testing.T) {
			want := deterministicPayload(tc.size)
			s := &uploadSpool{}
			defer s.Close()

			writeChunks(t, s, want, 64*1024)

			if s.size != int64(len(want)) {
				t.Errorf("spool size = %d, want %d", s.size, len(want))
			}

			reader, err := s.Reader()
			if err != nil {
				t.Fatalf("Reader: %v", err)
			}
			got, err := io.ReadAll(reader)
			if err != nil {
				t.Fatalf("ReadAll: %v", err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("round trip mismatch: got %d bytes, want %d", len(got), len(want))
			}
		})
	}
}

// TestUploadSpoolSpillsToDisk confirms large uploads actually leave memory —
// otherwise the fix would pass the round-trip test while still buffering
// everything in RAM.
func TestUploadSpoolSpillsToDisk(t *testing.T) {
	s := &uploadSpool{}
	defer s.Close()

	writeChunks(t, s, deterministicPayload(wsSpillThreshold+4096), 64*1024)

	if s.file == nil {
		t.Fatal("payload above the threshold stayed in memory; spilling to disk is the point of the spool")
	}
	if s.buf.Len() != 0 {
		t.Errorf("in-memory buffer still holds %d bytes after spilling", s.buf.Len())
	}
}

// TestUploadSpoolStaysInMemoryWhenSmall confirms small uploads avoid the temp
// file entirely, so the common case takes no filesystem hit.
func TestUploadSpoolStaysInMemoryWhenSmall(t *testing.T) {
	s := &uploadSpool{}
	defer s.Close()

	writeChunks(t, s, deterministicPayload(4096), 1024)

	if s.file != nil {
		t.Error("small payload created a temp file; it should stay in memory")
	}
}

// TestUploadSpoolCloseIsIdempotent guards the deferred cleanup path.
func TestUploadSpoolCloseIsIdempotent(t *testing.T) {
	s := &uploadSpool{}
	writeChunks(t, s, deterministicPayload(wsSpillThreshold+1), 64*1024)

	s.Close()
	s.Close() // must not panic on a second call

	if s.file != nil {
		t.Error("Close left the temp file handle set")
	}
}

// TestUploadSpoolReaderIsRewindable confirms Reader can be taken after writes
// without the caller having to seek, which is what the upload handler relies on.
func TestUploadSpoolReaderIsRewindable(t *testing.T) {
	want := deterministicPayload(wsSpillThreshold + 512)
	s := &uploadSpool{}
	defer s.Close()
	writeChunks(t, s, want, 32*1024)

	for i := 0; i < 2; i++ {
		reader, err := s.Reader()
		if err != nil {
			t.Fatalf("Reader call %d: %v", i, err)
		}
		got, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("ReadAll call %d: %v", i, err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("read %d returned %d bytes, want %d", i, len(got), len(want))
		}
	}
}
