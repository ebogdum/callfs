//go:build windows

package localfs

// No platform-specific timestamp extraction needed on Windows.
// extractPlatformMetadata in metadata_windows.go uses ModTime directly.
