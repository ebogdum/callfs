package main

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/ebogdum/callfs/v2/config"
	"go.uber.org/zap"
)

var streamingRoutePrefixes = []string{"/v1/files/", "/v1/shards/", "/v1/internal/shards/"}

func isStreamingRequest(r *http.Request) bool {
	switch r.Method {
	case http.MethodPut, http.MethodPost, http.MethodGet:
	default:
		return false
	}
	for _, prefix := range streamingRoutePrefixes {
		if strings.HasPrefix(r.URL.Path, prefix) {
			return true
		}
	}
	return false
}

func deadlineAt(now time.Time, timeout time.Duration) time.Time {
	if timeout <= 0 {
		return time.Time{}
	}
	return now.Add(timeout)
}

func withRequestDeadlines(next http.Handler, serverCfg *config.ServerConfig, logger *zap.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		readTimeout := serverCfg.ReadTimeout
		writeTimeout := serverCfg.WriteTimeout
		if isStreamingRequest(r) {
			readTimeout = serverCfg.TransferTimeout
			writeTimeout = serverCfg.TransferTimeout
		}
		now := time.Now()
		controller := http.NewResponseController(w)
		if err := controller.SetReadDeadline(deadlineAt(now, readTimeout)); err != nil && !errors.Is(err, http.ErrNotSupported) {
			logger.Warn("Failed to set request read deadline", zap.Error(err))
		}
		if err := controller.SetWriteDeadline(deadlineAt(now, writeTimeout)); err != nil && !errors.Is(err, http.ErrNotSupported) {
			logger.Warn("Failed to set request write deadline", zap.Error(err))
		}
		next.ServeHTTP(w, r)
	})
}
