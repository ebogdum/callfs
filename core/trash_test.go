package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/v2/auth"
	"github.com/ebogdum/callfs/v2/jobs"
	"github.com/ebogdum/callfs/v2/metadata"
)

type allowAllAuthorizer struct{}

func (allowAllAuthorizer) Authorize(context.Context, string, string, auth.PermissionType) error {
	return nil
}

func (allowAllAuthorizer) AuthorizeWithMetadata(context.Context, string, string, *metadata.Metadata, auth.PermissionType) error {
	return nil
}

type discardPublisher struct{}

func (discardPublisher) Publish(string, any) {}

func newTestTrash(t *testing.T, admins ...string) (*Engine, *Trash, *jobs.Runner) {
	t.Helper()
	engine := newTestEngine(t)
	runner := jobs.NewRunner(discardPublisher{}, zap.NewNop())
	trash := NewTrash(engine, allowAllAuthorizer{}, auth.NewAdminSet(admins), runner, time.Hour, zap.NewNop())
	return engine, trash, runner
}

func TestTrashMoveAndRestoreFile(t *testing.T) {
	ctx := context.Background()
	engine, trash, _ := newTestTrash(t)
	createFile(t, engine, "/a.txt", "hello")

	item, err := trash.MoveToTrash(ctx, "u1", "/a.txt")
	if nil != err {
		t.Fatalf("MoveToTrash: %v", err)
	}
	if _, err := engine.GetMetadata(ctx, "/a.txt"); !errors.Is(err, metadata.ErrNotFound) {
		t.Fatalf("original still present: %v", err)
	}
	page, err := trash.List(ctx, "u1", "", "", 10)
	if nil != err || 1 != len(page.Items) || item.ID != page.Items[0].ID || 5 != page.TotalBytes {
		t.Fatalf("List = %+v, %v", page, err)
	}
	if "/a.txt" != page.Items[0].OriginalPath || "u1" != page.Items[0].DeletedBy || 5 != page.Items[0].Size {
		t.Fatalf("item fields = %+v", page.Items[0])
	}

	restored, err := trash.Restore(ctx, "u1", item.ID, "", "")
	if nil != err || "/a.txt" != restored {
		t.Fatalf("Restore = %q, %v", restored, err)
	}
	if got := readFile(t, engine, "/a.txt"); "hello" != got {
		t.Fatalf("restored content = %q", got)
	}
	page, err = trash.List(ctx, "u1", "", "", 10)
	if nil != err || 0 != len(page.Items) {
		t.Fatalf("trash after restore = %+v, %v", page, err)
	}
}

func TestTrashRestoreConflicts(t *testing.T) {
	ctx := context.Background()
	engine, trash, _ := newTestTrash(t)

	createFile(t, engine, "/a.txt", "old")
	failItem, err := trash.MoveToTrash(ctx, "u1", "/a.txt")
	if nil != err {
		t.Fatalf("MoveToTrash: %v", err)
	}
	createFile(t, engine, "/a.txt", "new")
	if _, err := trash.Restore(ctx, "u1", failItem.ID, "", "fail"); !errors.Is(err, ErrTrashConflict) {
		t.Fatalf("fail conflict err = %v", err)
	}
	keptPath, err := trash.Restore(ctx, "u1", failItem.ID, "", "keep_both")
	if nil != err || "/a.txt (restored)" != keptPath {
		t.Fatalf("keep_both = %q, %v", keptPath, err)
	}
	if got := readFile(t, engine, keptPath); "old" != got {
		t.Fatalf("keep_both content = %q", got)
	}

	createFile(t, engine, "/b.txt", "gone")
	replaceItem, err := trash.MoveToTrash(ctx, "u1", "/b.txt")
	if nil != err {
		t.Fatalf("MoveToTrash b: %v", err)
	}
	createFile(t, engine, "/b.txt", "replacement")
	replacedPath, err := trash.Restore(ctx, "u1", replaceItem.ID, "", "replace")
	if nil != err || "/b.txt" != replacedPath {
		t.Fatalf("replace = %q, %v", replacedPath, err)
	}
	if got := readFile(t, engine, "/b.txt"); "gone" != got {
		t.Fatalf("replace content = %q", got)
	}
}

func TestTrashRestoreToAlternatePath(t *testing.T) {
	ctx := context.Background()
	engine, trash, _ := newTestTrash(t)
	createFile(t, engine, "/c.txt", "moved")
	item, err := trash.MoveToTrash(ctx, "u1", "/c.txt")
	if nil != err {
		t.Fatalf("MoveToTrash: %v", err)
	}
	restored, err := trash.Restore(ctx, "u1", item.ID, "/archive/deep/c.txt", "")
	if nil != err || "/archive/deep/c.txt" != restored {
		t.Fatalf("Restore to alternate = %q, %v", restored, err)
	}
	if got := readFile(t, engine, restored); "moved" != got {
		t.Fatalf("alternate content = %q", got)
	}
}

func TestTrashRejectsInvalidRequests(t *testing.T) {
	ctx := context.Background()
	engine, trash, _ := newTestTrash(t)
	if _, err := trash.MoveToTrash(ctx, "u1", "/.callfs/trash/x"); !errors.Is(err, ErrTrashInvalid) {
		t.Fatalf("trash path err = %v", err)
	}
	if _, err := trash.MoveToTrash(ctx, "u1", "/"); !errors.Is(err, ErrTrashInvalid) {
		t.Fatalf("root err = %v", err)
	}
	createFile(t, engine, "/d.txt", "d")
	item, err := trash.MoveToTrash(ctx, "u1", "/d.txt")
	if nil != err {
		t.Fatalf("MoveToTrash: %v", err)
	}
	if _, err := trash.Restore(ctx, "u1", "../etc", "", ""); !errors.Is(err, ErrTrashInvalid) {
		t.Fatalf("malformed id err = %v", err)
	}
	if _, err := trash.Restore(ctx, "u1", item.ID, "", "overwrite_all"); !errors.Is(err, ErrTrashInvalid) {
		t.Fatalf("bad on_conflict err = %v", err)
	}
	if _, err := trash.List(ctx, "u1", "", "not-a-cursor!", 10); !errors.Is(err, ErrTrashInvalid) {
		t.Fatalf("bad cursor err = %v", err)
	}
}

func TestTrashOwnerScope(t *testing.T) {
	ctx := context.Background()
	engine, trash, _ := newTestTrash(t, "admin1")
	createFile(t, engine, "/own.txt", "1")
	createFile(t, engine, "/other.txt", "2")
	mine, err := trash.MoveToTrash(ctx, "u1", "/own.txt")
	if nil != err {
		t.Fatalf("MoveToTrash own: %v", err)
	}
	theirs, err := trash.MoveToTrash(ctx, "u2", "/other.txt")
	if nil != err {
		t.Fatalf("MoveToTrash other: %v", err)
	}

	page, err := trash.List(ctx, "u1", "", "", 10)
	if nil != err || 1 != len(page.Items) || mine.ID != page.Items[0].ID {
		t.Fatalf("member list = %+v, %v", page, err)
	}
	if _, err := trash.List(ctx, "u1", "u2", "", 10); !errors.Is(err, auth.ErrPermissionDenied) {
		t.Fatalf("member filter other err = %v", err)
	}
	page, err = trash.List(ctx, "admin1", "", "", 10)
	if nil != err || 2 != len(page.Items) || 2 != page.TotalBytes {
		t.Fatalf("admin list = %+v, %v", page, err)
	}
	page, err = trash.List(ctx, "admin1", "u2", "", 10)
	if nil != err || 1 != len(page.Items) || theirs.ID != page.Items[0].ID {
		t.Fatalf("admin filter list = %+v, %v", page, err)
	}
	if err := trash.Delete(ctx, "u1", theirs.ID); !errors.Is(err, auth.ErrPermissionDenied) {
		t.Fatalf("delete other's item err = %v", err)
	}
	if err := trash.Delete(ctx, "admin1", theirs.ID); nil != err {
		t.Fatalf("admin delete err = %v", err)
	}
}

func TestTrashListPagination(t *testing.T) {
	ctx := context.Background()
	engine, trash, _ := newTestTrash(t)
	for _, name := range []string{"/p1.txt", "/p2.txt", "/p3.txt"} {
		createFile(t, engine, name, "x")
		if _, err := trash.MoveToTrash(ctx, "u1", name); nil != err {
			t.Fatalf("MoveToTrash %s: %v", name, err)
		}
	}
	first, err := trash.List(ctx, "u1", "", "", 2)
	if nil != err || 2 != len(first.Items) || "" == first.NextCursor || 3 != first.TotalBytes {
		t.Fatalf("first page = %+v, %v", first, err)
	}
	second, err := trash.List(ctx, "u1", "", first.NextCursor, 2)
	if nil != err || 1 != len(second.Items) || "" != second.NextCursor || 3 != second.TotalBytes {
		t.Fatalf("second page = %+v, %v", second, err)
	}
	if second.Items[0].ID == first.Items[0].ID || second.Items[0].ID == first.Items[1].ID {
		t.Fatal("pages overlap")
	}
}

func TestTrashDeleteForGood(t *testing.T) {
	ctx := context.Background()
	engine, trash, _ := newTestTrash(t)
	createFile(t, engine, "/gone.txt", "bye")
	item, err := trash.MoveToTrash(ctx, "u1", "/gone.txt")
	if nil != err {
		t.Fatalf("MoveToTrash: %v", err)
	}
	if err := trash.Delete(ctx, "u1", item.ID); nil != err {
		t.Fatalf("Delete: %v", err)
	}
	page, err := trash.List(ctx, "u1", "", "", 10)
	if nil != err || 0 != len(page.Items) || 0 != page.TotalBytes {
		t.Fatalf("list after delete = %+v, %v", page, err)
	}
	if _, err := trash.Restore(ctx, "u1", item.ID, "", ""); !errors.Is(err, metadata.ErrNotFound) {
		t.Fatalf("restore after delete err = %v", err)
	}
}

func TestTrashCleanupRemovesExpired(t *testing.T) {
	ctx := context.Background()
	engine, trash, _ := newTestTrash(t)
	createFile(t, engine, "/e1.txt", "1")
	createFile(t, engine, "/e2.txt", "2")
	for _, name := range []string{"/e1.txt", "/e2.txt"} {
		if _, err := trash.MoveToTrash(ctx, "u1", name); nil != err {
			t.Fatalf("MoveToTrash %s: %v", name, err)
		}
	}
	removed, err := trash.Cleanup(ctx, time.Hour)
	if nil != err || 0 != removed {
		t.Fatalf("Cleanup within retention = %d, %v", removed, err)
	}
	removed, err = trash.Cleanup(ctx, 0)
	if nil != err || 2 != removed {
		t.Fatalf("Cleanup expired = %d, %v", removed, err)
	}
	page, err := trash.List(ctx, "u1", "", "", 10)
	if nil != err || 0 != len(page.Items) {
		t.Fatalf("list after cleanup = %+v, %v", page, err)
	}
}

func TestTrashEmptyRunsJob(t *testing.T) {
	ctx := context.Background()
	engine, trash, runner := newTestTrash(t, "admin1")
	createFile(t, engine, "/f1.txt", "1")
	createFile(t, engine, "/f2.txt", "2")
	createFile(t, engine, "/f3.txt", "3")
	if _, err := trash.MoveToTrash(ctx, "u1", "/f1.txt"); nil != err {
		t.Fatalf("MoveToTrash f1: %v", err)
	}
	if _, err := trash.MoveToTrash(ctx, "u1", "/f2.txt"); nil != err {
		t.Fatalf("MoveToTrash f2: %v", err)
	}
	if _, err := trash.MoveToTrash(ctx, "u2", "/f3.txt"); nil != err {
		t.Fatalf("MoveToTrash f3: %v", err)
	}
	job, err := trash.Empty(ctx, "u1", "")
	if nil != err {
		t.Fatalf("Empty: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		current, err := runner.Get(job.ID)
		if nil != err {
			t.Fatalf("job get: %v", err)
		}
		if current.State.Terminal() {
			if jobs.StateSucceeded != current.State {
				t.Fatalf("job state = %s", current.State)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("job did not finish")
		}
		time.Sleep(10 * time.Millisecond)
	}
	page, err := trash.List(ctx, "admin1", "", "", 10)
	if nil != err || 1 != len(page.Items) || "u2" != page.Items[0].DeletedBy {
		t.Fatalf("remaining after empty = %+v, %v", page, err)
	}
}
