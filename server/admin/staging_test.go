package admin

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/v2/jobs"
)

func TestCleanLocalStagingSkipsOverwriteCopies(t *testing.T) {
	root := t.TempDir()
	aside := ".callfs-tmp-x.callfs-old-abc123"
	stale := ".callfs-tmp-stale"
	for _, name := range []string{aside, stale} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0600); nil != err {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	items, err := scanLocalStaging(root, time.Now().UTC())
	if nil != err {
		t.Fatalf("scan: %v", err)
	}
	for _, item := range items {
		if "/"+aside == item.Path {
			t.Fatalf("overwrite copy listed as staging: %s", item.Path)
		}
	}

	done := make(chan stagingCleanResult, 1)
	runner := jobs.NewRunner(nil, zap.NewNop())
	runner.Submit("staging.clean", jobsTestActor, func(ctx context.Context, p *jobs.Progress) (any, error) {
		result, err := cleanLocalStaging(ctx, root, 0, p, zap.NewNop())
		done <- result
		return result, err
	})
	select {
	case result := <-done:
		if 1 != result.Removed {
			t.Fatalf("removed %d entries, want 1", result.Removed)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("staging cleanup did not finish")
	}

	if _, err := os.Stat(filepath.Join(root, aside)); nil != err {
		t.Fatalf("overwrite copy was removed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, stale)); !os.IsNotExist(err) {
		t.Fatalf("stale entry kept: %v", err)
	}
}
