// Package log provides structured logging capabilities for CallFS using slog.
// It includes JSON logging with request ID context support and configurable log levels.
package log

import (
	"log/slog"
)

// Logger wraps slog.Logger with additional context support
type Logger struct {
	*slog.Logger
}
