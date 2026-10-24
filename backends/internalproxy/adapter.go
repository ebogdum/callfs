package internalproxy

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/metadata"
)

const (
	proxyRequestTimeout = 30 * time.Second
	maxDrainBytes       = 64 << 10
	maxListingBytes     = 64 << 20
	maxListingEntries   = 100000
)

type proxiedFileInfo struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	Type  string `json:"type"`
	Size  int64  `json:"size"`
	Mode  string `json:"mode"`
	Owner string `json:"owner"`
	MTime string `json:"mtime"`
}

// InternalProxyAdapter implements the backends.Storage interface by proxying requests
// to other CallFS instances for Local FS content
type InternalProxyAdapter struct {
	client            *http.Client
	mu                sync.RWMutex
	instanceMap       map[string]string // instanceID -> endpoint
	internalAuthToken string
	logger            *zap.Logger
}

// NewInternalProxyAdapter creates a new internal proxy adapter
var ErrRemoteConflict = errors.New("owning server refused the change because of a conflict")

func NewInternalProxyAdapter(peerEndpoints map[string]string, authToken string, skipTLSVerify bool, logger *zap.Logger) (*InternalProxyAdapter, error) {
	for instanceID, endpoint := range peerEndpoints {
		if err := validatePeerEndpoint(endpoint); nil != err {
			return nil, fmt.Errorf("peer %s: %w", instanceID, err)
		}
	}
	transport := sharedTransport(skipTLSVerify)
	var roundTripper http.RoundTripper = transport
	if skipTLSVerify {
		roundTripper = &loopbackOnlyRoundTripper{next: transport}
		logger.Warn("internal proxy TLS verification disabled; requests are limited to loopback peers")
	}

	// Configure HTTP client with optimized settings
	client := &http.Client{
		Transport: roundTripper,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	return &InternalProxyAdapter{
		client:            client,
		instanceMap:       peerEndpoints,
		internalAuthToken: authToken,
		logger:            logger,
	}, nil
}

var (
	secureTransportOnce   sync.Once
	secureTransport       *http.Transport
	insecureTransportOnce sync.Once
	insecureTransport     *http.Transport
)

func newTunedTransport() *http.Transport {
	return &http.Transport{
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     false,
		MaxIdleConns:          1024,
		MaxIdleConnsPerHost:   256,
		MaxConnsPerHost:       0,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		WriteBufferSize:       64 << 10,
		ReadBufferSize:        64 << 10,
		DisableCompression:    true,
	}
}

func sharedTransport(skipTLSVerify bool) *http.Transport {
	if skipTLSVerify {
		insecureTransportOnce.Do(func() {
			insecureTransport = newTunedTransport()
			insecureTransport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		})
		return insecureTransport
	}
	secureTransportOnce.Do(func() {
		secureTransport = newTunedTransport()
	})
	return secureTransport
}

type loopbackOnlyRoundTripper struct {
	next http.RoundTripper
}

func (rt *loopbackOnlyRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if !isLoopbackHost(req.URL.Hostname()) {
		if nil != req.Body {
			_ = req.Body.Close()
		}
		return nil, fmt.Errorf("peer %s is not loopback: TLS verification is disabled, so the internal auth token is only sent to loopback peers", req.URL.Host)
	}
	return rt.next.RoundTrip(req)
}

func isLoopbackHost(host string) bool {
	if "localhost" == strings.ToLower(host) {
		return true
	}
	ip := net.ParseIP(host)
	return nil != ip && ip.IsLoopback()
}

func validatePeerEndpoint(endpoint string) error {
	parsed, err := url.Parse(endpoint)
	if nil != err {
		return fmt.Errorf("invalid endpoint %q: %w", endpoint, err)
	}
	hostname := parsed.Hostname()
	if "" == hostname {
		return fmt.Errorf("endpoint %q has no host", endpoint)
	}
	if port := parsed.Port(); "" != port {
		portNumber, convErr := strconv.Atoi(port)
		if nil != convErr || 1 > portNumber || 65535 < portNumber {
			return fmt.Errorf("endpoint %q has invalid port", endpoint)
		}
	}
	if nil != parsed.User || "" != parsed.RawQuery || "" != parsed.Fragment || ("" != parsed.Path && "/" != parsed.Path) {
		return fmt.Errorf("endpoint %q must be scheme://host[:port] without credentials, path, query or fragment", endpoint)
	}
	if "https" == parsed.Scheme {
		return nil
	}
	if "http" == parsed.Scheme && isLoopbackHost(hostname) {
		return nil
	}
	return fmt.Errorf("endpoint %q must use https (http is allowed only for loopback peers)", endpoint)
}

func (a *InternalProxyAdapter) AddPeerEndpoint(instanceID, endpoint string) error {
	if err := validatePeerEndpoint(endpoint); nil != err {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.instanceMap[instanceID] = endpoint
	return nil
}

// getEndpoint returns the endpoint for an instance ID (thread-safe).
func (a *InternalProxyAdapter) getEndpoint(instanceID string) (string, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	endpoint, exists := a.instanceMap[instanceID]
	return endpoint, exists
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
	endpoint, exists := a.getEndpoint(instanceID)
	if !exists {
		return nil, fmt.Errorf("unknown instance ID: %s", instanceID)
	}

	// Construct request URL
	reqURL := buildProxyURL(endpoint, path)

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add internal authentication
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", a.internalAuthToken))

	a.logger.Debug("Proxying file open request",
		zap.String("instance_id", instanceID),
		zap.String("path", path),
		zap.String("url", reqURL))

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
	instanceID := a.getInstanceIDFromContext(ctx)
	if instanceID == "" {
		return fmt.Errorf("internal proxy requires instance ID in context")
	}
	_, err := a.CreateOnInstance(ctx, instanceID, path, reader, size, "", false)
	return err
}

func (a *InternalProxyAdapter) CreateOnInstance(ctx context.Context, instanceID, path string, reader io.Reader, size int64, ifMatch string, ifNoneMatch bool) (string, error) {
	endpoint, exists := a.getEndpoint(instanceID)
	if !exists {
		return "", fmt.Errorf("unknown instance ID: %s", instanceID)
	}

	reqURL := buildProxyURL(endpoint, path)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, reader)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", a.internalAuthToken))
	req.Header.Set("Content-Type", "application/octet-stream")
	setConditionalHeaders(req, ifMatch, ifNoneMatch)
	if size > 0 {
		req.ContentLength = size
	}

	a.logger.Debug("Proxying file create request",
		zap.String("instance_id", instanceID),
		zap.String("path", path),
		zap.String("url", reqURL))

	resp, err := a.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to proxy request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		if resp.StatusCode == http.StatusNotFound {
			return "", metadata.ErrNotFound
		}
		if resp.StatusCode == http.StatusConflict {
			return "", metadata.ErrAlreadyExists
		}
		if resp.StatusCode == http.StatusPreconditionFailed {
			return "", metadata.ErrPrecondition
		}
		return "", fmt.Errorf("proxy request failed with status %d", resp.StatusCode)
	}

	return remoteETag(resp), nil
}

func (a *InternalProxyAdapter) OpenRange(ctx context.Context, path string, offset, length int64) (io.ReadCloser, error) {
	instanceID := a.getInstanceIDFromContext(ctx)
	if instanceID == "" {
		return nil, fmt.Errorf("internal proxy requires instance ID in context")
	}
	endpoint, exists := a.getEndpoint(instanceID)
	if !exists {
		return nil, fmt.Errorf("unknown instance ID: %s", instanceID)
	}
	if length == 0 {
		return io.NopCloser(strings.NewReader("")), nil
	}

	rangeHeader := fmt.Sprintf("bytes=%d-", offset)
	if length > 0 {
		rangeHeader = fmt.Sprintf("bytes=%d-%d", offset, offset+length-1)
	}

	reqURL := buildProxyURL(endpoint, path)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", a.internalAuthToken))
	req.Header.Set("Range", rangeHeader)

	a.logger.Debug("Proxying file range open request",
		zap.String("instance_id", instanceID),
		zap.String("path", path),
		zap.String("range", rangeHeader),
		zap.String("url", reqURL))

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to proxy request: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		resp.Body.Close()
		if resp.StatusCode == http.StatusNotFound {
			return nil, metadata.ErrNotFound
		}
		return nil, fmt.Errorf("proxy request failed with status %d", resp.StatusCode)
	}

	return resp.Body, nil
}

// Update updates a file by proxying to the owning instance
func (a *InternalProxyAdapter) Update(ctx context.Context, path string, reader io.Reader, size int64) error {
	instanceID := a.getInstanceIDFromContext(ctx)
	if instanceID == "" {
		return fmt.Errorf("internal proxy requires instance ID in context")
	}
	_, err := a.UpdateOnInstance(ctx, instanceID, path, reader, size, "", false)
	return err
}

// UpdateOnInstance updates a file on a specific CallFS instance
func (a *InternalProxyAdapter) UpdateOnInstance(ctx context.Context, instanceID, path string, reader io.Reader, size int64, ifMatch string, ifNoneMatch bool) (string, error) {
	endpoint, exists := a.getEndpoint(instanceID)
	if !exists {
		return "", fmt.Errorf("unknown instance ID: %s", instanceID)
	}

	// Construct request URL
	reqURL := buildProxyURL(endpoint, path)

	req, err := http.NewRequestWithContext(ctx, "PUT", reqURL, reader)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Add internal authentication
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", a.internalAuthToken))
	req.Header.Set("Content-Type", "application/octet-stream")
	setConditionalHeaders(req, ifMatch, ifNoneMatch)
	if size > 0 {
		req.ContentLength = size
	}

	a.logger.Debug("Proxying file update request",
		zap.String("instance_id", instanceID),
		zap.String("path", path),
		zap.String("url", reqURL))

	resp, err := a.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to proxy request: %w", err)
	}
	defer drainAndClose(resp.Body)

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		if resp.StatusCode == http.StatusNotFound {
			return "", metadata.ErrNotFound
		}
		if resp.StatusCode == http.StatusPreconditionFailed {
			return "", metadata.ErrPrecondition
		}
		return "", fmt.Errorf("proxy request failed with status %d", resp.StatusCode)
	}

	return remoteETag(resp), nil
}

// Delete removes a file by proxying to the owning instance
func (a *InternalProxyAdapter) Delete(ctx context.Context, path string) error {
	instanceID := a.getInstanceIDFromContext(ctx)
	if instanceID == "" {
		return fmt.Errorf("internal proxy requires instance ID in context")
	}
	return a.DeleteOnInstance(ctx, instanceID, path)
}

// Move relocates a file by proxying a PATCH (move) request to the owning
// instance. The instance ID is taken from the context.
func (a *InternalProxyAdapter) Move(ctx context.Context, oldPath, newPath string) error {
	instanceID := a.getInstanceIDFromContext(ctx)
	if instanceID == "" {
		return fmt.Errorf("internal proxy requires instance ID in context")
	}
	body, err := json.Marshal(map[string]any{"destination": newPath})
	if err != nil {
		return fmt.Errorf("failed to encode move request: %w", err)
	}
	return a.MoveOnInstance(ctx, instanceID, oldPath, body)
}

// MoveOnInstance sends a PATCH (rename/move) request for srcPath to a specific
// CallFS instance. The body is a JSON move/rename payload; the target instance
// performs the operation against its own backend and the shared metadata store.
func (a *InternalProxyAdapter) MoveOnInstance(ctx context.Context, instanceID, srcPath string, body []byte) error {
	ctx, cancel := context.WithTimeout(ctx, proxyRequestTimeout)
	defer cancel()

	endpoint, exists := a.getEndpoint(instanceID)
	if !exists {
		return fmt.Errorf("unknown instance ID: %s", instanceID)
	}

	reqURL := buildProxyURL(endpoint, srcPath)
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, reqURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", a.internalAuthToken))
	req.Header.Set("Content-Type", "application/json")

	a.logger.Debug("Proxying file move request",
		zap.String("instance_id", instanceID),
		zap.String("path", srcPath),
		zap.String("url", reqURL))

	resp, err := a.client.Do(req)
	if nil != err {
		return &ProxyError{err: fmt.Errorf("failed to proxy request: %w", err)}
	}
	defer drainAndClose(resp.Body)

	if http.StatusOK != resp.StatusCode {
		if http.StatusNotFound == resp.StatusCode {
			return metadata.ErrNotFound
		}
		if http.StatusConflict == resp.StatusCode {
			return metadata.ErrAlreadyExists
		}
		if http.StatusBadRequest == resp.StatusCode {
			return &BadRequestError{message: "peer rejected move request"}
		}
		return &ProxyError{err: fmt.Errorf("proxy request failed with status %d", resp.StatusCode)}
	}
	return nil
}

type ProxyError struct {
	err error
}

func (e *ProxyError) Error() string {
	return e.err.Error()
}

func (e *ProxyError) Unwrap() error {
	return e.err
}

type BadRequestError struct {
	message string
}

func (e *BadRequestError) Error() string {
	return e.message
}

// DeleteOnInstance deletes a file on a specific CallFS instance
func (a *InternalProxyAdapter) DeleteOnInstance(ctx context.Context, instanceID, path string) error {
	return a.deleteOnInstance(ctx, instanceID, path, false)
}

func (a *InternalProxyAdapter) DeleteTreeOnInstance(ctx context.Context, instanceID, path string) error {
	return a.deleteOnInstance(ctx, instanceID, path, true)
}

func (a *InternalProxyAdapter) deleteOnInstance(ctx context.Context, instanceID, path string, recursive bool) error {
	ctx, cancel := context.WithTimeout(ctx, proxyRequestTimeout)
	defer cancel()

	endpoint, exists := a.getEndpoint(instanceID)
	if !exists {
		return fmt.Errorf("unknown instance ID: %s", instanceID)
	}

	// Construct request URL
	reqURL := buildProxyURL(endpoint, path)
	if recursive {
		reqURL += "?recursive=true"
	}

	req, err := http.NewRequestWithContext(ctx, "DELETE", reqURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Add internal authentication
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", a.internalAuthToken))

	a.logger.Debug("Proxying file delete request",
		zap.String("instance_id", instanceID),
		zap.String("path", path),
		zap.String("url", reqURL))

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to proxy request: %w", err)
	}
	defer resp.Body.Close()

	if http.StatusNoContent == resp.StatusCode || (recursive && http.StatusOK == resp.StatusCode) {
		return nil
	}
	if http.StatusNotFound == resp.StatusCode {
		return metadata.ErrNotFound
	}
	if http.StatusConflict == resp.StatusCode {
		return ErrRemoteConflict
	}
	return fmt.Errorf("proxy request failed with status %d", resp.StatusCode)
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
	ctx, cancel := context.WithTimeout(ctx, proxyRequestTimeout)
	defer cancel()

	endpoint, exists := a.getEndpoint(instanceID)
	if !exists {
		return nil, fmt.Errorf("unknown instance ID: %s", instanceID)
	}

	// Construct request URL
	reqURL := buildProxyURL(endpoint, path)

	req, err := http.NewRequestWithContext(ctx, "HEAD", reqURL, nil)
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

	size, err := strconv.ParseInt(resp.Header.Get("X-CallFS-Size"), 10, 64)
	if nil != err {
		return nil, fmt.Errorf("invalid X-CallFS-Size header: %w", err)
	}
	owner := resp.Header.Get("X-CallFS-Owner")
	typeHeader := resp.Header.Get("X-CallFS-Type")
	if typeHeader == "" {
		typeHeader = "file"
	}
	mTime, err := time.Parse("2006-01-02T15:04:05Z07:00", resp.Header.Get("X-CallFS-MTime"))
	if nil != err {
		return nil, fmt.Errorf("invalid X-CallFS-MTime header: %w", err)
	}
	mode := resp.Header.Get("X-CallFS-Mode")
	if "" == mode {
		mode = "0644"
		if "directory" == typeHeader {
			mode = "0755"
		}
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
		Owner:       owner,
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
	ctx, cancel := context.WithTimeout(ctx, proxyRequestTimeout)
	defer cancel()

	endpoint, exists := a.getEndpoint(instanceID)
	if !exists {
		return nil, fmt.Errorf("unknown instance ID: %s", instanceID)
	}

	// Construct request URL
	reqURL := buildProxyURL(endpoint, path)

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add internal authentication
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", a.internalAuthToken))
	req.Header.Set("Accept-Encoding", "gzip")

	a.logger.Debug("Proxying directory list request",
		zap.String("instance_id", instanceID),
		zap.String("path", path),
		zap.String("url", reqURL))

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

	var listingBody io.Reader = io.LimitReader(resp.Body, maxListingBytes)
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gz, err := gzip.NewReader(listingBody)
		if err != nil {
			return nil, fmt.Errorf("failed to decode directory listing response: %w", err)
		}
		defer gz.Close()
		listingBody = io.LimitReader(gz, maxListingBytes)
	}

	dec := json.NewDecoder(listingBody)
	children := make([]*metadata.Metadata, 0)
	tok, err := dec.Token()
	if err != nil {
		return nil, fmt.Errorf("failed to decode directory listing response: %w", err)
	}
	if tok == nil {
		return children, nil
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '[' {
		return nil, fmt.Errorf("failed to decode directory listing response: expected array")
	}
	for dec.More() {
		if len(children) >= maxListingEntries {
			return nil, fmt.Errorf("directory listing response exceeds %d entries", maxListingEntries)
		}
		var item proxiedFileInfo
		if err := dec.Decode(&item); err != nil {
			return nil, fmt.Errorf("failed to decode directory listing response: %w", err)
		}
		children = append(children, listingEntryMetadata(path, item))
	}
	if _, err := dec.Token(); err != nil {
		return nil, fmt.Errorf("failed to decode directory listing response: %w", err)
	}

	return children, nil
}

func listingEntryMetadata(path string, item proxiedFileInfo) *metadata.Metadata {
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

	return &metadata.Metadata{
		Name:        item.Name,
		Path:        itemPath,
		Type:        item.Type,
		Size:        item.Size,
		Mode:        mode,
		Owner:       item.Owner,
		MTime:       itemMTime,
		ATime:       itemMTime,
		CTime:       itemMTime,
		BackendType: "localfs",
	}
}

// CreateDirectory creates a directory by proxying to the target instance
func (a *InternalProxyAdapter) CreateDirectory(ctx context.Context, path string) error {
	instanceID := a.getInstanceIDFromContext(ctx)
	if instanceID == "" {
		return fmt.Errorf("internal proxy requires instance ID in context")
	}
	return a.CreateDirectoryOnInstance(ctx, instanceID, path)
}

func (a *InternalProxyAdapter) CreateDirectoryOnInstance(ctx context.Context, instanceID, path string) error {
	endpoint, exists := a.getEndpoint(instanceID)
	if !exists {
		return fmt.Errorf("unknown instance ID: %s", instanceID)
	}

	ctx, cancel := context.WithTimeout(ctx, proxyRequestTimeout)
	defer cancel()

	reqURL := strings.TrimRight(buildProxyURL(endpoint, path), "/") + "/"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", a.internalAuthToken))

	a.logger.Debug("Proxying directory create request",
		zap.String("instance_id", instanceID),
		zap.String("path", path),
		zap.String("url", reqURL))

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to proxy request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		if resp.StatusCode == http.StatusNotFound {
			return metadata.ErrNotFound
		}
		if resp.StatusCode == http.StatusConflict {
			return metadata.ErrAlreadyExists
		}
		return fmt.Errorf("proxy request failed with status %d", resp.StatusCode)
	}

	return nil
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

// buildProxyURL constructs a properly encoded URL for proxying to a peer instance.
// Uses url.JoinPath for correct per-segment encoding of multi-segment paths.
func buildProxyURL(endpoint, path string) string {
	base := strings.TrimRight(endpoint, "/")
	cleanPath := strings.TrimLeft(path, "/")
	result, err := url.JoinPath(base, "v1", "files", cleanPath)
	if err != nil {
		// Fallback: manual construction with per-segment encoding
		segments := strings.Split(cleanPath, "/")
		for i, seg := range segments {
			segments[i] = url.PathEscape(seg)
		}
		return base + "/v1/files/" + strings.Join(segments, "/")
	}
	return result
}

func setConditionalHeaders(req *http.Request, ifMatch string, ifNoneMatch bool) {
	switch {
	case ifMatch == "*":
		req.Header.Set("If-Match", "*")
	case ifMatch != "":
		req.Header.Set("If-Match", fmt.Sprintf(`"%s"`, ifMatch))
	}
	if ifNoneMatch {
		req.Header.Set("If-None-Match", "*")
	}
}

func drainAndClose(body io.ReadCloser) {
	io.Copy(io.Discard, io.LimitReader(body, maxDrainBytes))
	body.Close()
}

func remoteETag(resp *http.Response) string {
	return strings.Trim(resp.Header.Get("ETag"), `"`)
}

func (a *InternalProxyAdapter) Ping(ctx context.Context) error {
	a.mu.RLock()
	peers := make(map[string]string, len(a.instanceMap))
	for id, endpoint := range a.instanceMap {
		peers[id] = endpoint
	}
	a.mu.RUnlock()
	if 0 == len(peers) {
		return errors.New("no peer servers are configured")
	}
	var mu sync.Mutex
	var unreachable []string
	var wg sync.WaitGroup
	for id, endpoint := range peers {
		wg.Add(1)
		go func(id, endpoint string) {
			defer wg.Done()
			if err := a.pingPeer(ctx, endpoint); nil != err {
				mu.Lock()
				unreachable = append(unreachable, id)
				mu.Unlock()
			}
		}(id, endpoint)
	}
	wg.Wait()
	if 0 == len(unreachable) {
		return nil
	}
	sort.Strings(unreachable)
	return fmt.Errorf("%d of %d peer servers do not answer: %s", len(unreachable), len(peers), strings.Join(unreachable, ", "))
}

func (a *InternalProxyAdapter) pingPeer(ctx context.Context, endpoint string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(endpoint, "/")+"/health", nil)
	if nil != err {
		return err
	}
	resp, err := a.client.Do(req)
	if nil != err {
		return err
	}
	defer resp.Body.Close()
	if http.StatusOK != resp.StatusCode {
		return fmt.Errorf("answered %d", resp.StatusCode)
	}
	return nil
}
