package localfs

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ebogdum/callfs/v2/backends"
)

type SyncMode string

const (
	SyncFull    SyncMode = "full"
	SyncBarrier SyncMode = "barrier"
	SyncNone    SyncMode = "none"
)

var errExclusiveRenameUnsupported = errors.New("exclusive rename is not supported here")

func ParseSyncMode(value string) (SyncMode, error) {
	switch SyncMode(strings.ToLower(strings.TrimSpace(value))) {
	case "", SyncFull:
		return SyncFull, nil
	case SyncBarrier:
		return SyncBarrier, nil
	case SyncNone:
		return SyncNone, nil
	default:
		return "", fmt.Errorf("localfs sync mode must be one of: full, barrier, none (got %q)", value)
	}
}

type Option func(*LocalFSAdapter)

func WithSyncMode(mode SyncMode) Option {
	return func(a *LocalFSAdapter) {
		a.syncMode = mode
	}
}

type FlushConfig struct {
	Workers         int
	MaxPendingBytes int64
	MaxDelay        time.Duration
}

func WithWriteAck(ack backends.WriteAck) Option {
	return func(a *LocalFSAdapter) {
		a.ack = ack
	}
}

func WithFlush(cfg FlushConfig) Option {
	return func(a *LocalFSAdapter) {
		a.flushCfg = cfg
	}
}
