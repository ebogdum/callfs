package internalproxy

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/metadata"
)

type proxiedFileInfo struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	Type  string `json:"type"`
	Size  int64  `json:"size"`
	Mode  string `json:"mode"`
	UID   int    `json:"uid"`
	GID   int    `json:"gid"`
	MTime string `json:"mtime"`
}

// InternalProxyAdapter implements the backends.Storage interface by proxying requests
// to other CallFS instances for Local FS content
type InternalProxyAdapter struct {
	client            *http.Client
	instanceMap       map[string]string // instanceID -> endpoint
	internalAuthToken string
	logger            *zap.Logger
}

// NewInternalProxyAdapter creates a new internal proxy adapter
func NewInternalProxyAdapter(peerEndpoints map[string]string, authToken string, skipTLSVerify bool, logger *zap.Logger) (*InternalProxyAdapter, error) {
	// Configure HTTP transport with optional TLS skip verification
	transport := &http.Transport{
		MaxIdleConnsPerHost: 100,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  true, // Let the client handle compression
	}

	// Configure TLS settings if needed
	if skipTLSVerify {
		transport.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: true,
		}
	}

	// Configure HTTP client with optimized settings
	client := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}

	return &InternalProxyAdapter{
		client:            client,
		instanceMap:       peerEndpoints,
		internalAuthToken: authToken,
		logger:            logger,
	}, nil
}

// Open opens a file for reading by proxying to the owning instance
// This method expects the instance ID to be provided via context
func (a *InternalProxyAdapter) Open(ctx context.Context, path string) (io.ReadCloser, error) {
	instanceID := a.getInstanceIDFromContext(ctx)
	if instanceID == "" {
		return nil, fmt.Errorf("internal proxy requires instance ID in context")
	}
	return a.OpenFromInstance(ctx, instanceID, path)
}

// OpenFromInstance opens a file from a specific CallFS instance
func (a *InternalProxyAdapter) OpenFromInstance(ctx context.Context, instanceID, path string) (io.ReadCloser, error) {
	endpoint, exists := a.instanceMap[instanceID]
	if !exists {
		return nil, fmt.Errorf("unknown instance ID: %s", instanceID)
	}

	// Construct request URL
	url := fmt.Sprintf("%s/v1/files/%s", strings.TrimRight(endpoint, "/"), strings.TrimLeft(path, "/"))

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add internal authentication
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", a.internalAuthToken))

	a.logger.Debug("Proxying file open request",
		zap.String("instance_id", instanceID),
		zap.String("path", path),
		zap.String("url", url))

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to proxy request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		if resp.StatusCode == http.StatusNotFound {
			return nil, metadata.ErrNotFound
		}
		return nil, fmt.Errorf("proxy request failed with status %d", resp.StatusCode)
	}

	return resp.Body, nil
}

// Create creates a new file by proxying to the target instance
func (a *InternalProxyAdapter) Create(ctx context.Context, path string, reader io.Reader, size int64) error {
	return fmt.Errorf("Create method not supported for internal proxy - files are created locally")
}

// Update updates a file by proxying to the owning instance
func (a *InternalProxyAdapter) Update(ctx context.Context, path string, reader io.Reader, size int64) error {
	instanceID := a.getInstanceIDFromContext(ctx)
	if instanceID == "" {
		return fmt.Errorf("internal proxy requires instance ID in context")
	}
	return a.UpdateOnInstance(ctx, instanceID, path, reader, size)
}

// UpdateOnInstance updates a file on a specific CallFS instance
func (a *InternalProxyAdapter) UpdateOnInstance(ctx context.Context, instanceID, path string, reader io.Reader, size int64) error {
	endpoint, exists := a.instanceMap[instanceID]
	if !exists {
		return fmt.Errorf("unknown instance ID: %s", instanceID)
	}

	// Construct request URL
	url := fmt.Sprintf("%s/v1/files/%s", strings.TrimRight(endpoint, "/"), strings.TrimLeft(path, "/"))

	req, err := http.NewRequestWithContext(ctx, "PUT", url, reader)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Add internal authentication
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", a.internalAuthToken))
	req.Header.Set("Content-Type", "application/octet-stream")
	if size > 0 {
		req.ContentLength = size
	}

	a.logger.Debug("Proxying file update request",
		zap.String("instance_id", instanceID),
		zap.String("path", path),
		zap.String("url", url))

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to proxy request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		if resp.StatusCode == http.StatusNotFound {
			return metadata.ErrNotFound
		}
		return fmt.Errorf("proxy request failed with status %d", resp.StatusCode)
	}

	return nil
}

// Delete removes a file by proxying to the owning instance
func (a *InternalProxyAdapter) Delete(ctx context.Context, path string) error {
	instanceID := a.getInstanceIDFromContext(ctx)
	if instanceID == "" {
		return fmt.Errorf("internal proxy requires instance ID in context")
	}
	return a.DeleteOnInstance(ctx, instanceID, path)
}

// DeleteOnInstance deletes a file on a specific CallFS instance
func (a *InternalProxyAdapter) DeleteOnInstance(ctx context.Context, instanceID, path string) error {
	endpoint, exists := a.instanceMap[instanceID]
	if !exists {
		return fmt.Errorf("unknown instance ID: %s", instanceID)
	}

	// Construct request URL
	url := fmt.Sprintf("%s/v1/files/%s", strings.TrimRight(endpoint, "/"), strings.TrimLeft(path, "/"))

	req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Add internal authentication
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", a.internalAuthToken))

	a.logger.Debug("Proxying file delete request",
		zap.String("instance_id", instanceID),
		zap.String("path", path),
		zap.String("url", url))

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to proxy request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		if resp.StatusCode == http.StatusNotFound {
			return metadata.ErrNotFound
		}
		return fmt.Errorf("proxy request failed with status %d", resp.StatusCode)
	}

	return nil
}

// Stat returns metadata for a file by proxying to the owning instance
func (a *InternalProxyAdapter) Stat(ctx context.Context, path string) (*metadata.Metadata, error) {
	instanceID := a.getInstanceIDFromContext(ctx)
	if instanceID == "" {
		return nil, fmt.Errorf("internal proxy requires instance ID in context")
	}
	return a.StatOnInstance(ctx, instanceID, path)
}

// StatOnInstance gets file metadata from a specific CallFS instance
func (a *InternalProxyAdapter) StatOnInstance(ctx context.Context, instanceID, path string) (*metadata.Metadata, error) {
	endpoint, exists := a.instanceMap[instanceID]
	if !exists {
		return nil, fmt.Errorf("unknown instance ID: %s", instanceID)
	}

	// Construct request URL
	url := fmt.Sprintf("%s/v1/files/%s", strings.TrimRight(endpoint, "/"), strings.TrimLeft(path, "/"))

	req, err := http.NewRequestWithContext(ctx, "HEAD", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add internal authentication
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", a.internalAuthToken))

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to proxy request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusNotFound {
			return nil, metadata.ErrNotFound
		}
		return nil, fmt.Errorf("proxy request failed with status %d", resp.StatusCode)
	}

	size, _ := strconv.ParseInt(resp.Header.Get("X-CallFS-Size"), 10, 64)
	uid, _ := strconv.Atoi(resp.Header.Get("X-CallFS-UID"))
	gid, _ := strconv.Atoi(resp.Header.Get("X-CallFS-GID"))
	typeHeader := resp.Header.Get("X-CallFS-Type")
	if typeHeader == "" {
		typeHeader = "file"
	}
	mTime := time.Now()
	if mtimeHeader := resp.Header.Get("X-CallFS-MTime"); mtimeHeader != "" {
		if parsedMTime, err := time.Parse("2006-01-02T15:04:05Z07:00", mtimeHeader); err == nil {
			mTime = parsedMTime
		}
	}
	mode := resp.Header.Get("X-CallFS-Mode")
	if mode == "" {
		mode = "0644"
	}
	cleanPath := path
	if !strings.HasPrefix(cleanPath, "/") {
		cleanPath = "/" + cleanPath
	}
	name := filepath.Base(cleanPath)
	if cleanPath == "/" {
		name = "/"
	}

	return &metadata.Metadata{
		Name:        name,
		Path:        cleanPath,
		Type:        typeHeader,
		Size:        size,
		Mode:        mode,
		UID:         uid,
		GID:         gid,
		MTime:       mTime,
		ATime:       mTime,
		CTime:       mTime,
		BackendType: "localfs",
	}, nil
}

// ListDirectory lists directory contents by proxying to the owning instance
func (a *InternalProxyAdapter) ListDirectory(ctx context.Context, path string) ([]*metadata.Metadata, error) {
	instanceID := a.getInstanceIDFromContext(ctx)
	if instanceID == "" {
		return nil, fmt.Errorf("internal proxy requires instance ID in context")
	}
	return a.ListDirectoryOnInstance(ctx, instanceID, path)
}

// ListDirectoryOnInstance lists directory contents from a specific CallFS instance
func (a *InternalProxyAdapter) ListDirectoryOnInstance(ctx context.Context, instanceID, path string) ([]*metadata.Metadata, error) {
	endpoint, exists := a.instanceMap[instanceID]
	if !exists {
		return nil, fmt.Errorf("unknown instance ID: %s", instanceID)
	}

	// Construct request URL
	url := fmt.Sprintf("%s/v1/files/%s", strings.TrimRight(endpoint, "/"), strings.TrimLeft(path, "/"))

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add internal authentication
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", a.internalAuthToken))

	a.logger.Debug("Proxying directory list request",
		zap.String("instance_id", instanceID),
		zap.String("path", path),
		zap.String("url", url))

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to proxy request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusNotFound {
			return nil, metadata.ErrNotFound
		}
		return nil, fmt.Errorf("proxy request failed with status %d", resp.StatusCode)
	}

	var fileInfos []proxiedFileInfo
	if err := json.NewDecoder(resp.Body).Decode(&fileInfos); err != nil {
		return nil, fmt.Errorf("failed to decode directory listing response: %w", err)
	}

	children := make([]*metadata.Metadata, 0, len(fileInfos))
	for _, item := range fileInfos {
		itemPath := item.Path
		if itemPath == "" {
			itemPath = "/" + strings.TrimLeft(filepath.Join(path, item.Name), "/")
		}
		if !strings.HasPrefix(itemPath, "/") {
			itemPath = "/" + itemPath
		}

		itemMTime := time.Now()
		if item.MTime != "" {
			if parsedMTime, err := time.Parse("2006-01-02T15:04:05Z07:00", item.MTime); err == nil {
				itemMTime = parsedMTime
			}
		}

		mode := item.Mode
		if mode == "" {
			mode = "0644"
			if item.Type == "directory" {
				mode = "0755"
			}
		}

		children = append(children, &metadata.Metadata{
			Name:        item.Name,
			Path:        itemPath,
			Type:        item.Type,
			Size:        item.Size,
			Mode:        mode,
			UID:         item.UID,
			GID:         item.GID,
			MTime:       itemMTime,
			ATime:       itemMTime,
			CTime:       itemMTime,
			BackendType: "localfs",
		})
	}

	return children, nil
}

// CreateDirectory creates a directory by proxying to the target instance
func (a *InternalProxyAdapter) CreateDirectory(ctx context.Context, path string) error {
	return fmt.Errorf("CreateDirectory method not supported for internal proxy - directories are created locally")
}

// Close closes the HTTP client resources
func (a *InternalProxyAdapter) Close() error {
	a.client.CloseIdleConnections()
	return nil
}

// Context key for instance ID
type contextKey string

const instanceIDKey contextKey = "instance_id"

// getInstanceIDFromContext extracts the instance ID from context
func (a *InternalProxyAdapter) getInstanceIDFromContext(ctx context.Context) string {
	if instanceID, ok := ctx.Value(instanceIDKey).(string); ok {
		return instanceID
	}
	return ""
}

// WithInstanceID returns a new context with the instance ID
func WithInstanceID(ctx context.Context, instanceID string) context.Context {
	return context.WithValue(ctx, instanceIDKey, instanceID)
}
