package pathutil

import (
	"testing"

	"github.com/ebogdum/callfs/metadata"
)

func TestClean(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    string
		shouldError bool
	}{
		{
			name:     "empty path",
			input:    "",
			expected: "/",
		},
		{
			name:     "simple path",
			input:    "file.txt",
			expected: "/file.txt",
		},
		{
			name:     "nested path",
			input:    "dir/subdir/file.txt",
			expected: "/dir/subdir/file.txt",
		},
		{
			name:     "root path",
			input:    "/",
			expected: "/",
		},
		{
			name:        "absolute path escape",
			input:       "/etc/passwd",
			shouldError: true,
		},
		{
			name:        "directory traversal",
			input:       "../../../etc/passwd",
			shouldError: true,
		},
		{
			name:        "mixed traversal",
			input:       "dir/../../../etc/passwd",
			shouldError: true,
		},
		{
			name:     "safe relative navigation",
			input:    "dir/../file.txt",
			expected: "/file.txt",
		},
		{
			name:     "current directory",
			input:    "./file.txt",
			expected: "/file.txt",
		},
		{
			name:     "multiple slashes",
			input:    "dir//file.txt",
			expected: "/dir/file.txt",
		},
		{
			name:     "trailing slash",
			input:    "dir/",
			expected: "/dir",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Clean(tt.input)

			if tt.shouldError {
				if err == nil {
					t.Errorf("expected error for input %q, got none", tt.input)
				}
				if err != metadata.ErrForbidden {
					t.Errorf("expected ErrForbidden, got %v", err)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error for input %q: %v", tt.input, err)
				}
				if result != tt.expected {
					t.Errorf("for input %q, expected %q, got %q", tt.input, tt.expected, result)
				}
			}
		})
	}
}

func TestSafeJoin(t *testing.T) {
	tests := []struct {
		name        string
		root        string
		rel         string
		shouldError bool
	}{
		{
			name: "safe join",
			root: "/safe/root",
			rel:  "file.txt",
		},
		{
			name: "safe nested join",
			root: "/safe/root",
			rel:  "dir/subdir/file.txt",
		},
		{
			name:        "escape attempt",
			root:        "/safe/root",
			rel:         "../../../etc/passwd",
			shouldError: true,
		},
		{
			name:        "absolute path escape",
			root:        "/safe/root",
			rel:         "/etc/passwd",
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := SafeJoin(tt.root, tt.rel)

			if tt.shouldError {
				if err == nil {
					t.Errorf("expected error for root %q, rel %q, got none", tt.root, tt.rel)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error for root %q, rel %q: %v", tt.root, tt.rel, err)
				}
				// Basic check that result starts with root
				if result != "" && !hasPrefix(result, tt.root) {
					t.Errorf("result %q does not start with root %q", result, tt.root)
				}
			}
		})
	}
}

func TestValidatePath(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		shouldError bool
	}{
		{
			name:  "valid path",
			input: "valid/path/file.txt",
		},
		{
			name:        "empty path",
			input:       "",
			shouldError: true,
		},
		{
			name:        "null byte",
			input:       "file\x00.txt",
			shouldError: true,
		},
		{
			name:        "control character",
			input:       "file\x01.txt",
			shouldError: true,
		},
		{
			name:        "traversal attack",
			input:       "../../../etc/passwd",
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePath(tt.input)

			if tt.shouldError && err == nil {
				t.Errorf("expected error for input %q, got none", tt.input)
			}
			if !tt.shouldError && err != nil {
				t.Errorf("unexpected error for input %q: %v", tt.input, err)
			}
		})
	}
}

// helper function to check prefix (for compatibility)
func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[0:len(prefix)] == prefix
}
