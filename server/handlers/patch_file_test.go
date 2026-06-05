package handlers

import "testing"

func TestResolveDestinationRename(t *testing.T) {
	cases := []struct {
		src, name, want string
	}{
		{"/dir/a.txt", "b.txt", "/dir/b.txt"},
		{"/a.txt", "b.txt", "/b.txt"},
		{"/x/y/z", "renamed", "/x/y/renamed"},
	}
	for _, c := range cases {
		got, err := resolveDestination(c.src, renameMoveRequest{Name: c.name})
		if err != nil {
			t.Fatalf("resolveDestination(%q,%q): %v", c.src, c.name, err)
		}
		if got != c.want {
			t.Fatalf("resolveDestination(%q,%q) = %q, want %q", c.src, c.name, got, c.want)
		}
	}
}

func TestResolveDestinationRenameRejectsSlash(t *testing.T) {
	for _, name := range []string{"a/b", "..", "."} {
		if _, err := resolveDestination("/x.txt", renameMoveRequest{Name: name}); err == nil {
			t.Fatalf("expected error for name %q", name)
		}
	}
}

func TestResolveDestinationMoveNormalizes(t *testing.T) {
	cases := []struct {
		dst, want string
	}{
		{"/new/path.txt", "/new/path.txt"},
		{"relative/path.txt", "/relative/path.txt"},
		{"/dir/", "/dir"},
	}
	for _, c := range cases {
		got, err := resolveDestination("/src.txt", renameMoveRequest{Destination: c.dst})
		if err != nil {
			t.Fatalf("resolveDestination dest %q: %v", c.dst, err)
		}
		if got != c.want {
			t.Fatalf("resolveDestination dest %q = %q, want %q", c.dst, got, c.want)
		}
	}
}
