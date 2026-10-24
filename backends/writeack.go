package backends

import (
	"context"
	"fmt"
	"strings"
)

type WriteAck string

const (
	WriteAckMemory WriteAck = "memory"
	WriteAckDisk   WriteAck = "disk"
	WriteAckHeader          = "X-CallFS-Write-Ack"
)

type writeAckKey struct{}

func ParseWriteAck(value string) (WriteAck, error) {
	switch WriteAck(strings.ToLower(strings.TrimSpace(value))) {
	case "", WriteAckMemory:
		return WriteAckMemory, nil
	case WriteAckDisk:
		return WriteAckDisk, nil
	default:
		return "", fmt.Errorf("write ack must be one of: memory, disk (got %q)", value)
	}
}

func WithWriteAck(ctx context.Context, ack WriteAck) context.Context {
	return context.WithValue(ctx, writeAckKey{}, ack)
}

func WriteAckFrom(ctx context.Context, fallback WriteAck) WriteAck {
	if ack, ok := ctx.Value(writeAckKey{}).(WriteAck); ok && "" != ack {
		return ack
	}
	return fallback
}
