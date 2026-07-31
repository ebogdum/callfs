package handlers

import "testing"

// TestParseFilePathCanonicalizes is the regression guard for a path-confusion
// privilege escalation.
//
// ParseFilePath used to validate the path but then build FullPath from the raw
// request string. Every metadata store keys on that exact string, while the
// storage backends run it through pathutil.Clean (via SafeJoin) before touching
// disk. The two therefore disagreed for any non-canonical path:
//
//	PUT /v1/files/dir/../secret.txt
//	  metadata key -> "/dir/../secret.txt"  (no such entry: no ErrAlreadyExists,
//	                                         and Authorize falls through to a
//	                                         non-existent parent and allows)
//	  backend key  -> "<root>/secret.txt"   (overwrites the real file)
//
// Any authenticated user could destroy any other user's file that way, and
// leave an orphaned metadata row behind. FullPath must be canonical so both
// sides agree and ownership checks land on the real resource.
func TestParseFilePathCanonicalizes(t *testing.T) {
	tests := []struct {
		name           string
		urlPath        string
		wantFullPath   string
		wantParentPath string
		wantName       string
	}{
		{"dot-dot rewinds into parent", "dir/../secret.txt", "/secret.txt", "/", "secret.txt"},
		{"leading current-dir", "./secret.txt", "/secret.txt", "/", "secret.txt"},
		{"interior current-dir", "a/./b", "/a/b", "/a", "b"},
		{"duplicate separators", "a//b", "/a/b", "/a", "b"},
		{"many duplicate separators", "a///b////c", "/a/b/c", "/a/b", "c"},
		{"trailing current-dir segment", "a/b/.", "/a/b", "/a", "b"},
		{"deep rewind back to same file", "a/b/../../secret.txt", "/secret.txt", "/", "secret.txt"},
		{"already canonical is unchanged", "a/b/c.txt", "/a/b/c.txt", "/a/b", "c.txt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseFilePath(tt.urlPath)
			if got.IsInvalid {
				t.Fatalf("ParseFilePath(%q) marked valid path invalid", tt.urlPath)
			}
			if got.FullPath != tt.wantFullPath {
				t.Errorf("FullPath = %q, want %q (metadata and backend must agree)", got.FullPath, tt.wantFullPath)
			}
			if got.ParentPath != tt.wantParentPath {
				t.Errorf("ParentPath = %q, want %q", got.ParentPath, tt.wantParentPath)
			}
			if got.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", got.Name, tt.wantName)
			}
		})
	}
}

// TestParseFilePathCanonicalDirectories checks the same canonicalization for
// directory paths, where the trailing slash must survive normalization.
func TestParseFilePathCanonicalDirectories(t *testing.T) {
	tests := []struct {
		urlPath      string
		wantFullPath string
		wantName     string
	}{
		{"a/./b/", "/a/b/", "b"},
		{"a//b/", "/a/b/", "b"},
		{"dir/../other/", "/other/", "other"},
	}

	for _, tt := range tests {
		got := ParseFilePath(tt.urlPath)
		if got.IsInvalid {
			t.Fatalf("ParseFilePath(%q) marked valid directory path invalid", tt.urlPath)
		}
		if !got.IsDirectory {
			t.Errorf("ParseFilePath(%q).IsDirectory = false, want true", tt.urlPath)
		}
		if got.FullPath != tt.wantFullPath {
			t.Errorf("ParseFilePath(%q).FullPath = %q, want %q", tt.urlPath, got.FullPath, tt.wantFullPath)
		}
		if got.Name != tt.wantName {
			t.Errorf("ParseFilePath(%q).Name = %q, want %q", tt.urlPath, got.Name, tt.wantName)
		}
	}
}

// TestParseFilePathRejectsEscapes confirms paths that genuinely escape the root
// are still rejected outright rather than silently clamped to "/".
func TestParseFilePathRejectsEscapes(t *testing.T) {
	for _, p := range []string{
		"../etc/passwd",
		"a/../../etc/passwd",
		"..",
		"a/../..",
		"foo\x00bar",
		"back\\slash",
	} {
		if got := ParseFilePath(p); !got.IsInvalid {
			t.Errorf("ParseFilePath(%q) = %+v, want IsInvalid=true", p, got)
		}
	}
}

// TestParseFilePathRootForms checks the various spellings of the root path all
// collapse to "/" and stay valid.
func TestParseFilePathRootForms(t *testing.T) {
	for _, p := range []string{"", "/", ".", "./", "a/.."} {
		got := ParseFilePath(p)
		if got.IsInvalid {
			t.Errorf("ParseFilePath(%q) marked root form invalid", p)
			continue
		}
		if got.FullPath != "/" {
			t.Errorf("ParseFilePath(%q).FullPath = %q, want %q", p, got.FullPath, "/")
		}
		if !got.IsDirectory {
			t.Errorf("ParseFilePath(%q).IsDirectory = false, want true", p)
		}
	}
}
