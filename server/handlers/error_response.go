package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/auth"
	"github.com/ebogdum/callfs/metadata"
)

// ErrorResponse represents a standardized error response
type ErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
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

	// Map specific errors to HTTP status codes and error codes (using errors.Is for wrapped errors)
	switch {
	case errors.Is(err, metadata.ErrNotFound):
		statusCode = http.StatusNotFound
		errorCode = "FILE_NOT_FOUND"
	case errors.Is(err, metadata.ErrAlreadyExists):
		statusCode = http.StatusConflict
		errorCode = "FILE_ALREADY_EXISTS"
	case errors.Is(err, auth.ErrAuthenticationFailed):
		statusCode = http.StatusUnauthorized
		errorCode = "AUTHENTICATION_FAILED"
	case errors.Is(err, auth.ErrPermissionDenied):
		statusCode = http.StatusForbidden
		errorCode = "PERMISSION_DENIED"
	default:
		statusCode = defaultStatusCode
		errorCode = "INTERNAL_ERROR"
	}

	w.WriteHeader(statusCode)

	message := err.Error()
	if errorCode == "INTERNAL_ERROR" {
		message = "an internal error occurred"
	}

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

	logLevel := logger.Info
	if statusCode >= 500 {
		logLevel = logger.Error
	}
	logLevel("Error response sent",
		zap.String("error_code", errorCode),
		zap.Int("status_code", statusCode),
		zap.Error(err))
}

// SendJSONResponse sends a JSON response with any data structure.
// Marshals to a buffer first so that encoding errors don't produce malformed responses.
func SendJSONResponse(w http.ResponseWriter, data interface{}) {
	buf, err := json.Marshal(data)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"Failed to encode response"}`))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(buf)
	_, _ = w.Write([]byte("\n"))
}
