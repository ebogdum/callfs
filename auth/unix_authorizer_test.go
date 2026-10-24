package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ebogdum/callfs/v2/metadata"
)

// stubStore is a minimal in-memory metadata store for testing the authorizer.
type stubStore struct {
	entries map[string]*metadata.Metadata
}

func newStubStore() *stubStore {
	return &stubStore{entries: make(map[string]*metadata.Metadata)}
}

func (s *stubStore) Get(_ context.Context, path string) (*metadata.Metadata, error) {
	if md, ok := s.entries[path]; ok {
		return md, nil
	}
	return nil, metadata.ErrNotFound
}
func (s *stubStore) Create(_ context.Context, md *metadata.Metadata) error {
	s.entries[md.Path] = md
	return nil
}
func (s *stubStore) Update(_ context.Context, _ *metadata.Metadata) error { return nil }
func (s *stubStore) Delete(_ context.Context, _ string) error             { return nil }
func (s *stubStore) Rename(_ context.Context, _, _ string) error          { return nil }
func (s *stubStore) ListChildren(_ context.Context, _ string) ([]*metadata.Metadata, error) {
	return nil, nil
}
func (s *stubStore) UpdateIfMatch(_ context.Context, _ *metadata.Metadata, _ string) error {
	return nil
}
func (s *stubStore) ListByPrefix(_ context.Context, _, _ string, _ int) ([]*metadata.Metadata, error) {
	return nil, nil
}
func (s *stubStore) GetSingleUseLink(_ context.Context, _ string) (*metadata.SingleUseLink, error) {
	return nil, metadata.ErrNotFound
}
func (s *stubStore) CreateSingleUseLink(_ context.Context, _ *metadata.SingleUseLink) error {
	return nil
}
func (s *stubStore) CountActiveSingleUseLinks(_ context.Context, _ string, _ time.Time) (int, error) {
	return 0, nil
}
func (s *stubStore) UpdateSingleUseLink(_ context.Context, _ string, _ string, _ *time.Time, _ *string) error {
	return nil
}
func (s *stubStore) CleanupExpiredLinks(_ context.Context, _ time.Time) (int, error) { return 0, nil }
func (s *stubStore) CleanupUsedLinks(_ context.Context, _ time.Time) (int, error)    { return 0, nil }
func (s *stubStore) Close() error                                                    { return nil }
func (s *stubStore) HasChildren(_ context.Context, _ string) (bool, error)           { return false, nil }
func (s *stubStore) ListDescendants(_ context.Context, _ string, _, _ int) ([]*metadata.Metadata, error) {
	return nil, nil
}
func (s *stubStore) ConsumeSingleUseLink(_ context.Context, _ string, _ string, _ time.Time) (*metadata.SingleUseLink, bool, error) {
	return nil, false, nil
}
func (s *stubStore) ListSingleUseLinks(_ context.Context, _ metadata.LinkFilter) ([]metadata.SingleUseLink, string, error) {
	return nil, "", nil
}
func (s *stubStore) GetSingleUseLinkByID(_ context.Context, _ int64) (*metadata.SingleUseLink, error) {
	return nil, metadata.ErrNotFound
}
func (s *stubStore) RevokeSingleUseLink(_ context.Context, _ int64) error { return nil }
func (s *stubStore) SearchMetadata(_ context.Context, _ metadata.SearchQuery) (metadata.SearchResult, error) {
	return metadata.SearchResult{}, nil
}
func (s *stubStore) OwnerStats(_ context.Context, _ string, _ int) ([]metadata.OwnerStat, string, error) {
	return nil, "", nil
}
func (s *stubStore) CountEmptyETags(_ context.Context) (int, error)  { return 0, nil }
func (s *stubStore) CountEmptyOwners(_ context.Context) (int, error) { return 0, nil }
func (s *stubStore) ListErasureInfos(_ context.Context, _, _ string, _ int) ([]*metadata.ErasureFileInfo, string, error) {
	return nil, "", nil
}

func TestOwnerHasFullAccess(t *testing.T) {
	store := newStubStore()
	store.entries["/myfile.txt"] = &metadata.Metadata{
		Path:  "/myfile.txt",
		Type:  "file",
		Owner: "api-user-0",
		Mode:  "0644",
	}

	az := NewUnixAuthorizer(store, nil)
	ctx := context.Background()

	for _, perm := range []PermissionType{ReadPerm, WritePerm, DeletePerm} {
		if err := az.Authorize(ctx, "api-user-0", "/myfile.txt", perm); err != nil {
			t.Errorf("owner should have %v permission, got: %v", perm, err)
		}
	}
}

func TestNonOwnerCanRead(t *testing.T) {
	store := newStubStore()
	store.entries["/myfile.txt"] = &metadata.Metadata{
		Path:  "/myfile.txt",
		Type:  "file",
		Owner: "api-user-0",
		Mode:  "0644",
	}

	az := NewUnixAuthorizer(store, nil)
	ctx := context.Background()

	if err := az.Authorize(ctx, "api-user-1", "/myfile.txt", ReadPerm); err != nil {
		t.Errorf("non-owner should have read access, got: %v", err)
	}
}

func TestNonOwnerCannotWrite(t *testing.T) {
	store := newStubStore()
	store.entries["/myfile.txt"] = &metadata.Metadata{
		Path:  "/myfile.txt",
		Type:  "file",
		Owner: "api-user-0",
		Mode:  "0644",
	}

	az := NewUnixAuthorizer(store, nil)
	ctx := context.Background()

	err := az.Authorize(ctx, "api-user-1", "/myfile.txt", WritePerm)
	if !errors.Is(err, ErrPermissionDenied) {
		t.Errorf("non-owner should be denied write, got: %v", err)
	}
}

func TestNonOwnerCannotDelete(t *testing.T) {
	store := newStubStore()
	store.entries["/myfile.txt"] = &metadata.Metadata{
		Path:  "/myfile.txt",
		Type:  "file",
		Owner: "api-user-0",
		Mode:  "0644",
	}

	az := NewUnixAuthorizer(store, nil)
	ctx := context.Background()

	err := az.Authorize(ctx, "api-user-1", "/myfile.txt", DeletePerm)
	if !errors.Is(err, ErrPermissionDenied) {
		t.Errorf("non-owner should be denied delete, got: %v", err)
	}
}

func TestRootUserBypassesPermissions(t *testing.T) {
	store := newStubStore()
	store.entries["/restricted.txt"] = &metadata.Metadata{
		Path:  "/restricted.txt",
		Type:  "file",
		Owner: "api-user-5",
		Mode:  "0600",
	}

	az := NewUnixAuthorizer(store, nil)
	ctx := context.Background()

	for _, perm := range []PermissionType{ReadPerm, WritePerm, DeletePerm} {
		if err := az.Authorize(ctx, "root", "/restricted.txt", perm); err != nil {
			t.Errorf("root should bypass permissions for %v, got: %v", perm, err)
		}
	}
}

func TestInternalProxyBypassesPermissions(t *testing.T) {
	store := newStubStore()
	store.entries["/restricted.txt"] = &metadata.Metadata{
		Path:  "/restricted.txt",
		Type:  "file",
		Owner: "api-user-5",
		Mode:  "0600",
	}

	az := NewUnixAuthorizer(store, nil)
	ctx := context.Background()

	for _, perm := range []PermissionType{ReadPerm, WritePerm, DeletePerm} {
		if err := az.Authorize(ctx, "internal-proxy", "/restricted.txt", perm); err != nil {
			t.Errorf("internal-proxy should bypass permissions for %v, got: %v", perm, err)
		}
	}
}

func TestNotFoundReturnsError(t *testing.T) {
	store := newStubStore()
	az := NewUnixAuthorizer(store, nil)
	ctx := context.Background()

	err := az.Authorize(ctx, "api-user-0", "/nonexistent.txt", ReadPerm)
	if !errors.Is(err, metadata.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestWriteToNonexistentChecksParent(t *testing.T) {
	store := newStubStore()
	store.entries["/mydir"] = &metadata.Metadata{
		Path:  "/mydir",
		Type:  "directory",
		Owner: "api-user-0",
		Mode:  "0755",
	}

	az := NewUnixAuthorizer(store, nil)
	ctx := context.Background()

	// Owner of parent can write new file
	if err := az.Authorize(ctx, "api-user-0", "/mydir/newfile.txt", WritePerm); err != nil {
		t.Errorf("owner of parent should be able to create child, got: %v", err)
	}

	// Non-owner of parent CAN also write (create children) in a directory
	if err := az.Authorize(ctx, "api-user-1", "/mydir/newfile.txt", WritePerm); err != nil {
		t.Errorf("any authenticated user should be able to create child in a directory, got: %v", err)
	}
}

func TestAuthorizeWithMetadataMissingFileChecksParent(t *testing.T) {
	store := newStubStore()
	store.entries["/mydir"] = &metadata.Metadata{
		Path:  "/mydir",
		Type:  "directory",
		Owner: "api-user-0",
		Mode:  "0755",
	}
	store.entries["/myfile.txt"] = &metadata.Metadata{
		Path:  "/myfile.txt",
		Type:  "file",
		Owner: "api-user-0",
		Mode:  "0644",
	}

	az := NewUnixAuthorizer(store, nil)
	ctx := context.Background()

	if err := az.AuthorizeWithMetadata(ctx, "api-user-1", "/mydir/newfile.txt", nil, WritePerm); nil != err {
		t.Errorf("missing file in writable directory should pass parent check, got: %v", err)
	}

	err := az.AuthorizeWithMetadata(ctx, "api-user-1", "/nonexistent.txt", nil, ReadPerm)
	if !errors.Is(err, metadata.ErrNotFound) {
		t.Errorf("missing file with ReadPerm should return ErrNotFound, got: %v", err)
	}

	err = az.AuthorizeWithMetadata(ctx, "api-user-1", "/myfile.txt", store.entries["/myfile.txt"], WritePerm)
	if !errors.Is(err, ErrPermissionDenied) {
		t.Errorf("non-owner write on existing file should be denied, got: %v", err)
	}
}

func TestDirectoryPermissions(t *testing.T) {
	store := newStubStore()
	store.entries["/shared"] = &metadata.Metadata{
		Path:  "/shared",
		Type:  "directory",
		Owner: "api-user-0",
		Mode:  "0755",
	}

	az := NewUnixAuthorizer(store, nil)
	ctx := context.Background()

	// Any user can read a directory
	if err := az.Authorize(ctx, "api-user-5", "/shared", ReadPerm); err != nil {
		t.Errorf("any user should read directory, got: %v", err)
	}

	// Any user can write (create children) in a directory
	if err := az.Authorize(ctx, "api-user-5", "/shared", WritePerm); err != nil {
		t.Errorf("any user should write to directory, got: %v", err)
	}

	// Only owner can delete a directory
	err := az.Authorize(ctx, "api-user-5", "/shared", DeletePerm)
	if !errors.Is(err, ErrPermissionDenied) {
		t.Errorf("non-owner should not delete directory, got: %v", err)
	}

	// Owner can delete
	if err := az.Authorize(ctx, "api-user-0", "/shared", DeletePerm); err != nil {
		t.Errorf("owner should delete directory, got: %v", err)
	}
}

func TestOwnerFieldIsAppUserString(t *testing.T) {
	// Verify that Owner is a plain app-user string, not a numeric ID
	store := newStubStore()
	store.entries["/test.txt"] = &metadata.Metadata{
		Path:  "/test.txt",
		Type:  "file",
		Owner: "api-user-42",
		Mode:  "0644",
	}

	az := NewUnixAuthorizer(store, nil)
	ctx := context.Background()

	// Exact string match required
	if err := az.Authorize(ctx, "api-user-42", "/test.txt", WritePerm); err != nil {
		t.Errorf("exact owner string should match, got: %v", err)
	}

	// Similar but different string must fail
	err := az.Authorize(ctx, "api-user-4", "/test.txt", WritePerm)
	if !errors.Is(err, ErrPermissionDenied) {
		t.Errorf("partial owner string should not match, got: %v", err)
	}
}

type stubAliases map[string][]string

func (s stubAliases) LegacyAliases(userID string) []string {
	return s[userID]
}

func TestLegacyAliasOwnerHasWriteAndDelete(t *testing.T) {
	store := newStubStore()
	store.entries["/legacy.txt"] = &metadata.Metadata{
		Path:  "/legacy.txt",
		Type:  "file",
		Owner: "api-user-0",
		Mode:  "0644",
	}

	az := NewUnixAuthorizer(store, stubAliases{"api-user-hashed-a": {"api-user-0"}})
	ctx := context.Background()

	for _, perm := range []PermissionType{WritePerm, DeletePerm} {
		if err := az.Authorize(ctx, "api-user-hashed-a", "/legacy.txt", perm); err != nil {
			t.Errorf("aliased identity should have %v permission, got: %v", perm, err)
		}
	}
}

func TestLegacyAliasDoesNotMatchOtherIdentity(t *testing.T) {
	store := newStubStore()
	store.entries["/legacy.txt"] = &metadata.Metadata{
		Path:  "/legacy.txt",
		Type:  "file",
		Owner: "api-user-0",
		Mode:  "0644",
	}

	az := NewUnixAuthorizer(store, stubAliases{"api-user-hashed-a": {"api-user-0"}, "api-user-hashed-b": {"api-user-1"}})
	ctx := context.Background()

	for _, perm := range []PermissionType{WritePerm, DeletePerm} {
		err := az.Authorize(ctx, "api-user-hashed-b", "/legacy.txt", perm)
		if !errors.Is(err, ErrPermissionDenied) {
			t.Errorf("non-aliased identity should be denied %v, got: %v", perm, err)
		}
	}
}
