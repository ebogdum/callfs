package handlers

import (
	"strings"

	"github.com/ebogdum/callfs/internal/pathutil"
)

// PathInfo represents parsed path information
type PathInfo struct {
	FullPath    string // The complete path from URL (e.g., "/some/path/here/and/file")
	ParentPath  string // The parent directory path (e.g., "/some/path/here/and")
	Name        string // The file or directory name (e.g., "file" or "dir")
	IsDirectory bool   // True if path ends with "/" indicating directory
	IsInvalid   bool   // True when path failed validation and should be rejected
}

// ParseFilePath extracts path information from a URL path according to new rules:
// 1. /files/ prefix is ignored and not part of the file path
// 2. If URL path ends in "/", it's a directory
// 3. If URL path doesn't end in "/", it's a file
// 4. /files/some/path/here/and/file -> path: "some/path/here/and", name: "file" (file)
// 5. /files/some/path/here/and/dir/ -> path: "some/path/here/and", name: "dir" (directory)
// SECURITY: Sanitizes path traversal attempts using secure path validation
func ParseFilePath(urlPath string) PathInfo {
	// Remove leading slash if present
	urlPath = strings.TrimPrefix(urlPath, "/")

	// Determine if it's a directory based on trailing slash
	isDirectory := strings.HasSuffix(urlPath, "/")

	// Remove trailing slash for processing
	cleanPath := strings.TrimSuffix(urlPath, "/")

	// Handle root case
	if cleanPath == "" || cleanPath == "." {
		return PathInfo{
			FullPath:    "/",
			ParentPath:  "/",
			Name:        "",
			IsDirectory: true, // Root is always a directory
		}
	}

	if strings.Contains(cleanPath, "\\") {
		return PathInfo{
			FullPath:    "/",
			ParentPath:  "/",
			Name:        "",
			IsDirectory: true,
			IsInvalid:   true,
		}
	}

	// SECURITY: Validate the relative path to prevent traversal attacks
	// Note: We work with relative paths here, SafeJoin in backends handles root joining
	if err := pathutil.ValidatePath(cleanPath); err != nil {
		// Mark invalid path so callers can return 400 Bad Request.
		return PathInfo{
			FullPath:    "/",
			ParentPath:  "/",
			Name:        "",
			IsDirectory: true,
			IsInvalid:   true,
		}
	}

	// Build full path - always start with /
	fullPath := "/" + cleanPath
	if isDirectory {
		fullPath += "/"
	}

	// Extract parent path and name
	parentPath := "/"
	name := cleanPath

	if lastSlash := strings.LastIndex(cleanPath, "/"); lastSlash >= 0 {
		if lastSlash == 0 {
			parentPath = "/"
		} else {
			parentPath = "/" + cleanPath[:lastSlash]
		}
		name = cleanPath[lastSlash+1:]
	}

	return PathInfo{
		FullPath:    fullPath,
		ParentPath:  parentPath,
		Name:        name,
		IsDirectory: isDirectory,
	}
}
