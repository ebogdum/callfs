package localfs

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
)

func TestOpenRangeMiddle(t *testing.T) {
	a, _ := newTestAdapter(t)
	ctx := context.Background()
	if err := a.Create(ctx, "r.txt", bytes.NewReader([]byte("0123456789")), 10); err != nil {
		t.Fatalf("create: %v", err)
	}

	rc, err := a.OpenRange(ctx, "r.txt", 2, 3)
	if err != nil {
		t.Fatalf("open range: %v", err)
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != "234" {
		t.Fatalf("expected 234, got %q", data)
	}
}

func TestOpenRangeToEnd(t *testing.T) {
	a, _ := newTestAdapter(t)
	ctx := context.Background()
	if err := a.Create(ctx, "r.txt", bytes.NewReader([]byte("0123456789")), 10); err != nil {
		t.Fatalf("create: %v", err)
	}

	rc, err := a.OpenRange(ctx, "r.txt", 7, -1)
	if err != nil {
		t.Fatalf("open range: %v", err)
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != "789" {
		t.Fatalf("expected 789, got %q", data)
	}
}

func TestOpenRangePastSize(t *testing.T) {
	a, _ := newTestAdapter(t)
	ctx := context.Background()
	if err := a.Create(ctx, "r.txt", bytes.NewReader([]byte("0123456789")), 10); err != nil {
		t.Fatalf("create: %v", err)
	}

	rc, err := a.OpenRange(ctx, "r.txt", 11, -1)
	if err == nil {
		rc.Close()
		t.Fatal("expected error for offset past size")
	}
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("expected io.ErrUnexpectedEOF, got %v", err)
	}
}
