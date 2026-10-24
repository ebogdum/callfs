package handlers

import (
	"errors"
	"testing"
)

func TestParseByteRange(t *testing.T) {
	tests := []struct {
		name       string
		header     string
		size       int64
		wantOffset int64
		wantLength int64
		wantOK     bool
		wantErr    bool
	}{
		{"middle", "bytes=2-5", 10, 2, 4, true, false},
		{"open ended", "bytes=4-", 10, 4, 6, true, false},
		{"suffix", "bytes=-3", 10, 7, 3, true, false},
		{"suffix larger than file", "bytes=-30", 10, 0, 10, true, false},
		{"end past size is clamped", "bytes=5-99", 10, 5, 5, true, false},
		{"single byte", "bytes=0-0", 10, 0, 1, true, false},
		{"multiple ranges ignored", "bytes=0-1,4-5", 10, 0, 0, false, false},
		{"other unit ignored", "items=0-1", 10, 0, 0, false, false},
		{"malformed ignored", "bytes=abc", 10, 0, 0, false, false},
		{"reversed ignored", "bytes=5-2", 10, 0, 0, false, false},
		{"empty header ignored", "", 10, 0, 0, false, false},
		{"start at size unsatisfiable", "bytes=10-", 10, 0, 0, false, true},
		{"start past size unsatisfiable", "bytes=20-30", 10, 0, 0, false, true},
		{"zero suffix unsatisfiable", "bytes=-0", 10, 0, 0, false, true},
		{"range on empty file unsatisfiable", "bytes=0-", 0, 0, 0, false, true},
		{"suffix on empty file unsatisfiable", "bytes=-1", 0, 0, 0, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			offset, length, ok, err := parseByteRange(tt.header, tt.size)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && !errors.Is(err, errRangeNotSatisfiable) {
				t.Fatalf("err = %v, want errRangeNotSatisfiable", err)
			}
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && (offset != tt.wantOffset || length != tt.wantLength) {
				t.Fatalf("offset, length = %d, %d, want %d, %d", offset, length, tt.wantOffset, tt.wantLength)
			}
		})
	}
}
