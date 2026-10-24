package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/v2/auth"
	"github.com/ebogdum/callfs/v2/backends/localfs"
	"github.com/ebogdum/callfs/v2/erasure"
	"github.com/ebogdum/callfs/v2/links"
	"github.com/ebogdum/callfs/v2/locks"
	"github.com/ebogdum/callfs/v2/metadata"
)

// ErrorResponse represents a standardized error response
type ErrorResponse struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

var clientErrorCodes = map[int]string{
	http.StatusBadRequest:                   "BAD_REQUEST",
	http.StatusNotFound:                     "NOT_FOUND",
	http.StatusConflict:                     "CONFLICT",
	http.StatusPreconditionFailed:           "PRECONDITION_FAILED",
	http.StatusRequestEntityTooLarge:        "PAYLOAD_TOO_LARGE",
	http.StatusRequestedRangeNotSatisfiable: "RANGE_NOT_SATISFIABLE",
	http.StatusTooManyRequests:              "RATE_LIMIT_EXCEEDED",
}

// customError is a simple error type for custom error messages
type customError struct {
	message string
}

func (e *customError) Error() string {
	return e.message
}

// SendErrorResponse sends a standardized JSON error response
func SendErrorResponse(w http.ResponseWriter, logger *zap.Logger, err error, defaultStatusCode int) {
	w.Header().Set("Content-Type", "application/json")

	var statusCode int
	var errorCode string
	var message string

	switch {
	case errors.Is(err, metadata.ErrNotFound):
		statusCode = http.StatusNotFound
		errorCode = "FILE_NOT_FOUND"
		message = "file not found"
	case errors.Is(err, metadata.ErrAlreadyExists):
		statusCode = http.StatusConflict
		errorCode = "FILE_ALREADY_EXISTS"
		message = "file already exists"
	case errors.Is(err, metadata.ErrSubtreeTooLarge):
		statusCode = http.StatusRequestEntityTooLarge
		errorCode = "SUBTREE_TOO_LARGE"
		message = "directory subtree too large"
	case errors.Is(err, metadata.ErrNameTooLong):
		statusCode = http.StatusBadRequest
		errorCode = "NAME_TOO_LONG"
		message = "file or directory name too long"
	case errors.Is(err, localfs.ErrInsufficientStorage):
		statusCode = http.StatusInsufficientStorage
		errorCode = "INSUFFICIENT_STORAGE"
		message = "insufficient storage space"
	case errors.Is(err, metadata.ErrPrecondition):
		statusCode = http.StatusPreconditionFailed
		errorCode = "PRECONDITION_FAILED"
		message = "precondition failed"
	case errors.Is(err, locks.ErrLockHeld):
		statusCode = http.StatusConflict
		errorCode = "RESOURCE_LOCKED"
		message = "resource is locked, retry"
	case errors.Is(err, auth.ErrAuthenticationFailed):
		statusCode = http.StatusUnauthorized
		errorCode = "AUTHENTICATION_FAILED"
		message = "authentication failed"
	case errors.Is(err, auth.ErrPermissionDenied):
		statusCode = http.StatusForbidden
		errorCode = "PERMISSION_DENIED"
		message = "permission denied"
	case errors.Is(err, erasure.ErrShardNotFound):
		statusCode = http.StatusNotFound
		errorCode = "SHARD_NOT_FOUND"
		message = "shard not found"
	case errors.Is(err, links.ErrLinkNotFound):
		statusCode = defaultStatusCode
		errorCode = "LINK_NOT_FOUND"
		message = err.Error()
	case errors.Is(err, links.ErrLinkExpired):
		statusCode = defaultStatusCode
		errorCode = "LINK_EXPIRED"
		message = err.Error()
	case errors.Is(err, links.ErrLinkInvalid):
		statusCode = defaultStatusCode
		errorCode = "LINK_INVALID"
		message = err.Error()
	default:
		statusCode = defaultStatusCode
		errorCode, message = genericErrorFields(statusCode, err)
	}

	w.WriteHeader(statusCode)

	response := ErrorResponse{
		Code:    errorCode,
		Message: message,
	}

	jsonBytes, marshalErr := json.Marshal(response)
	if marshalErr != nil {
		logger.Error("Failed to marshal error response", zap.Error(marshalErr))
		return
	}
	if _, writeErr := w.Write(jsonBytes); writeErr != nil {
		logger.Error("Failed to write error response", zap.Error(writeErr))
	}

	logLevel := logger.Debug
	if 500 <= statusCode && http.StatusNotImplemented != statusCode && !errors.Is(err, context.Canceled) {
		logLevel = logger.Error
	}
	logLevel("Error response sent",
		zap.String("error_code", errorCode),
		zap.Int("status_code", statusCode),
		zap.Error(err))
}

func SendJSONResponse(w http.ResponseWriter, data interface{}) {
	buf, err := json.Marshal(data)
	if nil != err {
		zap.L().Error("Failed to marshal JSON response", zap.Error(err))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"code":"INTERNAL_ERROR","message":"an internal error occurred"}` + "\n"))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(buf)
	_, _ = w.Write([]byte("\n"))
}

func genericErrorFields(statusCode int, err error) (string, string) {
	if http.StatusNotImplemented == statusCode {
		return "NOT_IMPLEMENTED", "this server is not set up for this request"
	}
	if 500 <= statusCode && http.StatusBadGateway != statusCode {
		return "INTERNAL_ERROR", "an internal error occurred"
	}
	errorCode, known := clientErrorCodes[statusCode]
	if !known {
		errorCode = strings.ToUpper(strings.ReplaceAll(http.StatusText(statusCode), " ", "_"))
	}
	if nil == err {
		return errorCode, http.StatusText(statusCode)
	}
	return errorCode, err.Error()
}
