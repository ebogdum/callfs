package handlers

import (
	"encoding/json"
	"fmt"
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

	// Map specific errors to HTTP status codes and error codes
	switch err {
	case metadata.ErrNotFound:
		statusCode = http.StatusNotFound
		errorCode = "FILE_NOT_FOUND"
	case metadata.ErrAlreadyExists:
		statusCode = http.StatusConflict
		errorCode = "FILE_ALREADY_EXISTS"
	case auth.ErrAuthenticationFailed:
		statusCode = http.StatusUnauthorized
		errorCode = "AUTHENTICATION_FAILED"
	case auth.ErrPermissionDenied:
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

	if err := json.NewEncoder(w).Encode(response); err != nil {
		logger.Error("Failed to encode error response", zap.Error(err))
		// Fallback to plain text
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintf(w, "Internal error occurred")
	}

	logger.Info("Error response sent",
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
