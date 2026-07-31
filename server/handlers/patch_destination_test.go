package handlers

import "testing"

// TestResolveDestinationCanonicalizes guards the move (`destination`) branch of
// PATCH /v1/files. These paths arrive in the request body and never pass through
// ParseFilePath, so without canonicalization here they reintroduce the exact
// metadata/backend split that ParseFilePath was fixed to prevent:
//
//	PATCH /v1/files/mine.txt  {"destination": "/dir/../victim.txt"}
//	  authorize -> "/dir/../victim.txt" (no such entry; parent "/dir/.." does not
//	               exist, so the ownership check falls through and allows)
//	  backend   -> "<root>/victim.txt"  (overwrites another user's file)
func TestResolveDestinationCanonicalizes(t *testing.T) {
	tests := []struct {
		name string
		src  string
		req  renameMoveRequest
		want string
	}{
		{"dot-dot in destination", "/mine.txt", renameMoveRequest{Destination: "/dir/../victim.txt"}, "/victim.txt"},
		{"current-dir in destination", "/mine.txt", renameMoveRequest{Destination: "/./victim.txt"}, "/victim.txt"},
		{"duplicate separators", "/mine.txt", renameMoveRequest{Destination: "/a//b//c.txt"}, "/a/b/c.txt"},
		{"relative destination", "/mine.txt", renameMoveRequest{Destination: "a/./b.txt"}, "/a/b.txt"},
		{"trailing slash trimmed", "/mine.txt", renameMoveRequest{Destination: "/a/b/"}, "/a/b"},
		{"deep rewind", "/mine.txt", renameMoveRequest{Destination: "/a/b/../../victim.txt"}, "/victim.txt"},
		{"already canonical", "/mine.txt", renameMoveRequest{Destination: "/a/b.txt"}, "/a/b.txt"},
		{"rename stays in parent", "/dir/mine.txt", renameMoveRequest{Name: "renamed.txt"}, "/dir/renamed.txt"},
		{"rename at root", "/mine.txt", renameMoveRequest{Name: "renamed.txt"}, "/renamed.txt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveDestination(tt.src, tt.req)
			if err != nil {
				t.Fatalf("resolveDestination(%q, %+v) returned error: %v", tt.src, tt.req, err)
			}
			if got != tt.want {
				t.Errorf("resolveDestination(%q, %+v) = %q, want %q", tt.src, tt.req, got, tt.want)
			}
		})
	}
}

// TestResolveDestinationRejectsEscapes confirms destinations that leave the root
// or carry hostile characters are refused rather than silently clamped.
func TestResolveDestinationRejectsEscapes(t *testing.T) {
	tests := []struct {
		name string
		req  renameMoveRequest
	}{
		{"parent escape", renameMoveRequest{Destination: "../etc/passwd"}},
		{"deep escape", renameMoveRequest{Destination: "/a/../../etc/passwd"}},
		{"bare dot-dot", renameMoveRequest{Destination: ".."}},
		{"null byte", renameMoveRequest{Destination: "/a\x00b"}},
		{"control character", renameMoveRequest{Destination: "/a\x01b"}},
		{"bidi override", renameMoveRequest{Destination: "/a‮b"}},
		{"empty destination and name", renameMoveRequest{}},
		{"name with separator", renameMoveRequest{Name: "a/b"}},
		{"name dot-dot", renameMoveRequest{Name: ".."}},
		{"name with null byte", renameMoveRequest{Name: "a\x00b"}},
		{"name with control char", renameMoveRequest{Name: "a\x01b"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveDestination("/mine.txt", tt.req)
			if err == nil {
				t.Errorf("resolveDestination(%+v) = %q, want error", tt.req, got)
			}
		})
	}
}
