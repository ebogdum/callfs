package backends

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

const TrueOverwriteHeader = "X-CallFS-True-Overwrite"

type trueOverwriteKey struct{}

func ParseTrueOverwrite(value string) (bool, error) {
	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	if nil != err {
		return false, fmt.Errorf("%s must be true or false (got %q)", TrueOverwriteHeader, value)
	}
	return parsed, nil
}

func WithTrueOverwrite(ctx context.Context, enabled bool) context.Context {
	return context.WithValue(ctx, trueOverwriteKey{}, enabled)
}

func TrueOverwriteFrom(ctx context.Context, fallback bool) bool {
	if enabled, ok := ctx.Value(trueOverwriteKey{}).(bool); ok {
		return enabled
	}
	return fallback
}

func UseTrueOverwrite(ctx context.Context, configured bool, tmpSaveMaxSize, size int64) bool {
	if enabled, ok := ctx.Value(trueOverwriteKey{}).(bool); ok {
		return enabled
	}
	if configured {
		return true
	}
	return 0 < tmpSaveMaxSize && 0 <= size && tmpSaveMaxSize < size
}
