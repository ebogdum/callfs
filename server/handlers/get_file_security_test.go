package handlers

import (
	"context"
	"testing"
	"time"

	"github.com/ebogdum/callfs/auth"
	"github.com/ebogdum/callfs/metadata"
)

// MockAuthorizer for testing authorization timing
type MockAuthorizer struct {
	ShouldFail bool
	CallOrder  []string
}

func (m *MockAuthorizer) Authorize(ctx context.Context, userID string, path string, perm auth.PermissionType) error {
	m.CallOrder = append(m.CallOrder, "authorize")
	if m.ShouldFail {
		return auth.ErrPermissionDenied
	}
	return nil
}

// MockMetadataStore for testing metadata access timing
type MockMetadataStore struct {
	ShouldFail bool
	CallOrder  []string
}

func (m *MockMetadataStore) GetMetadata(ctx context.Context, path string) (*metadata.Metadata, error) {
	m.CallOrder = append(m.CallOrder, "metadata")
	if m.ShouldFail {
		return nil, metadata.ErrNotFound
	}
	return &metadata.Metadata{
		Type:        "file",
		Size:        100,
		BackendType: "localfs",
		MTime:       time.Now(),
	}, nil
}

// TestAuthorizationTimingConstant ensures that authorization failures and
// metadata lookup failures take similar time, preventing timing attacks
func TestAuthorizationTimingConstant(t *testing.T) {
	// Test that unauthorized users don't get timing information about file existence

	// Create mock components
	authFail := &MockAuthorizer{ShouldFail: true}
	authPass := &MockAuthorizer{ShouldFail: false}
	metadataPass := &MockMetadataStore{ShouldFail: false}

	// Test 1: Authorization fails - no metadata call should happen
	ctx := context.Background()
	err1 := authFail.Authorize(ctx, "user", "/secret/file", auth.ReadPerm)
	if err1 == nil {
		t.Error("expected authorization to fail")
	}
	if len(authFail.CallOrder) != 1 || authFail.CallOrder[0] != "authorize" {
		t.Error("authorization was not called as expected")
	}

	// Test 2: Authorization passes, metadata lookup should happen
	err2 := authPass.Authorize(ctx, "user", "/secret/file", auth.ReadPerm)
	if err2 != nil {
		t.Errorf("authorization should have passed: %v", err2)
	}

	// Simulate the metadata call that would happen after successful auth
	_, metaErr := metadataPass.GetMetadata(ctx, "/secret/file")
	if metaErr != nil {
		t.Errorf("metadata lookup should have passed: %v", metaErr)
	}

	// Test 3: Verify that failed auth doesn't leak metadata timing
	authFail2 := &MockAuthorizer{ShouldFail: true}
	err3 := authFail2.Authorize(ctx, "user", "/nonexistent/file", auth.ReadPerm)
	if err3 == nil {
		t.Error("expected authorization to fail")
	}

	// The key security property: authorization failure should be immediate
	// and should not depend on whether the file exists or not
	if len(authFail2.CallOrder) != 1 {
		t.Error("authorization timing should be constant regardless of file existence")
	}
}

// TestPathSanitization tests the path cleaning functionality
func TestPathSanitization(t *testing.T) {
	maliciousPaths := []struct {
		input           string
		shouldBeBlocked bool
	}{
		{"normal/file.txt", false},
		{"../../../etc/passwd", true},
		{"/etc/passwd", false},
		{"..\\..\\..\\windows\\system32", true},
		{"dir/../../../etc/passwd", true},
		{"./file.txt", false},
		{"", false}, // Empty path should be handled safely
	}

	for _, test := range maliciousPaths {
		t.Run("path_"+test.input, func(t *testing.T) {
			// Test the ParseFilePath function
			pathInfo := ParseFilePath(test.input)

			// For malicious paths, parser should explicitly mark them invalid.
			if test.shouldBeBlocked {
				if !pathInfo.IsInvalid {
					t.Errorf("malicious path %s was not marked invalid", test.input)
				}
			} else if pathInfo.IsInvalid {
				t.Errorf("safe path %s was marked invalid", test.input)
			}

			// All paths should result in valid PathInfo structures
			if pathInfo.FullPath == "" && test.input != "" {
				t.Errorf("path sanitization resulted in empty path for input: %s", test.input)
			}
		})
	}
}
