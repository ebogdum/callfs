// Package pathutil provides secure path handling utilities for CallFS.
package pathutil

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ebogdum/callfs/metadata"
)

// Clean sanitizes a file path to prevent directory traversal attacks.
// It performs the following security checks:
// 1. Rejects absolute paths that could escape the root
// 2. Cleans path traversal sequences like "../"
// 3. Ensures the cleaned path doesn't escape the root boundary
// 4. Normalizes the path for consistent handling
func Clean(path string) (string, error) {
	if path == "" {
		return "/", nil
	}

	// Reject absolute paths that might escape root
	if filepath.IsAbs(path) && path != "/" {
		return "", metadata.ErrForbidden
	}

	// Prepare the path for cleaning by ensuring it starts with /
	pathToClean := "/" + strings.TrimPrefix(path, "/")

	// Clean the path to resolve any ".." or "." components
	cleaned := filepath.Clean(pathToClean)

	// Ensure the cleaned path is still within bounds
	if !strings.HasPrefix(cleaned, "/") {
		return "", metadata.ErrForbidden
	}

	// Security check: if the cleaned path tries to go above root, reject it
	// This happens when we have more ".." than directory levels
	if cleaned == "/" {
		return cleaned, nil
	}

	// Check if the path escaped the root by going up too many levels
	// We'll simulate the path resolution to see if it stays within bounds
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	depth := 0

	for _, part := range parts {
		if part == "" || part == "." {
			continue // Skip empty parts and current directory references
		}
		if part == ".." {
			depth--
			if depth < 0 {
				// Trying to go above root level
				return "", metadata.ErrForbidden
			}
		} else {
			depth++
		}
	}

	return cleaned, nil
}

// SafeJoin safely joins a root path with a relative path, ensuring
// the result stays within the root directory boundary.
// Returns an error if the path would escape the root.
func SafeJoin(root, rel string) (string, error) {
	// Clean both paths
	cleanRoot := filepath.Clean(root)

	cleanRel, err := Clean(rel)
	if err != nil {
		return "", err
	}

	// Join the paths
	joined := filepath.Join(cleanRoot, strings.TrimPrefix(cleanRel, "/"))

	// Ensure the result is still within the root
	// Use EvalSymlinks to resolve any symbolic links and check the real path
	resolved, err := filepath.EvalSymlinks(joined)
	if err != nil {
		// File might not exist yet — resolve the parent directory to detect symlink escapes
		dir := filepath.Dir(joined)
		if dir != cleanRoot {
			resolvedDir, dirErr := filepath.EvalSymlinks(dir)
			if dirErr == nil {
				relDir, relErr := filepath.Rel(cleanRoot, resolvedDir)
				if relErr != nil || strings.HasPrefix(relDir, "..") {
					return "", metadata.ErrForbidden
				}
				// Reconstruct using the resolved parent to prevent TOCTOU symlink swap
				return filepath.Join(resolvedDir, filepath.Base(joined)), nil
			}
		}
		// Parent is root or unresolvable — validate the joined path structurally
		relPath, relErr := filepath.Rel(cleanRoot, joined)
		if relErr != nil || strings.HasPrefix(relPath, "..") {
			return "", metadata.ErrForbidden
		}
		return joined, nil
	}

	// Symlinks fully resolved — verify the resolved path is within root
	relPath, relErr := filepath.Rel(cleanRoot, resolved)
	if relErr != nil || strings.HasPrefix(relPath, "..") {
		return "", metadata.ErrForbidden
	}

	// Return the resolved path to prevent TOCTOU symlink race
	return resolved, nil
}

// ValidatePath performs comprehensive path validation for security.
// It checks for common attack patterns and ensures the path is safe to use.
func ValidatePath(path string) error {
	if path == "" {
		return fmt.Errorf("path cannot be empty")
	}

	// Check for null bytes (can be used to bypass file extension checks)
	if strings.Contains(path, "\x00") {
		return metadata.ErrForbidden
	}

	// Check for control characters and Unicode BiDi overrides
	for _, char := range path {
		if char < 32 && char != '\t' {
			return metadata.ErrForbidden
		}
		// Block Unicode bidirectional override characters (log injection risk)
		if char >= 0x202A && char <= 0x202E {
			return metadata.ErrForbidden
		}
		if char >= 0x2066 && char <= 0x2069 {
			return metadata.ErrForbidden
		}
	}

	// Clean and validate the path
	_, err := Clean(path)
	return err
}
