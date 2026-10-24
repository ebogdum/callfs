package localfs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

const (
	overwriteNewMarker = ".callfs-new-"
	overwriteOldMarker = ".callfs-old-"
	overwriteNewTagLen = 32
	maxEntryNameLen    = 255
)

type Overwrite struct {
	adapter *LocalFSAdapter
	tmp     *tempFile
	dirRel  string
	rel     string
	oldRel  string
}

type OverwriteLeftover struct {
	Path   string
	Target string
	ETag   string
	Old    bool
}

func validOverwriteTag(tag string) bool {
	if "" == tag {
		return false
	}
	for _, r := range tag {
		if !('0' <= r && '9' >= r || 'a' <= r && 'z' >= r || 'A' <= r && 'Z' >= r || '-' == r || '_' == r) {
			return false
		}
	}
	return true
}

func overwriteName(base, marker, tag string) string {
	return "." + base + marker + tag
}

func parseOverwriteName(name string) (target, tag string, old, ok bool) {
	if !strings.HasPrefix(name, ".") {
		return "", "", false, false
	}
	oldAt := strings.LastIndex(name, overwriteOldMarker)
	newAt := strings.LastIndex(name, overwriteNewMarker)
	at, marker := newAt, overwriteNewMarker
	if oldAt > newAt {
		at, marker, old = oldAt, overwriteOldMarker, true
	}
	if 1 >= at {
		return "", "", false, false
	}
	tag = name[at+len(marker):]
	if !validOverwriteTag(tag) {
		return "", "", false, false
	}
	return name[1:at], tag, old, true
}

func IsOverwriteName(name string) bool {
	_, _, _, ok := parseOverwriteName(name)
	return ok
}

func (a *LocalFSAdapter) SupportsOverwriteAside(p, oldETag string) bool {
	rel, err := relPath(p)
	if nil != err || "." == rel || !validOverwriteTag(oldETag) {
		return false
	}
	base := path.Base(filepath.ToSlash(rel))
	return maxEntryNameLen >= len(overwriteName(base, overwriteOldMarker, oldETag)) &&
		maxEntryNameLen >= len(base)+1+len(overwriteNewMarker)+overwriteNewTagLen
}

func newOverwriteName(base string) func() (string, error) {
	return func() (string, error) {
		var buf [overwriteNewTagLen / 2]byte
		if _, err := rand.Read(buf[:]); nil != err {
			return "", err
		}
		return overwriteName(base, overwriteNewMarker, hex.EncodeToString(buf[:])), nil
	}
}

func (a *LocalFSAdapter) WriteOverwrite(ctx context.Context, p string, reader io.Reader, size int64) (*Overwrite, error) {
	rel, err := relPath(p)
	if nil != err {
		return nil, err
	}
	if err := a.space.ensure(a.rootPath, size); nil != err {
		return nil, err
	}
	dirRel, err := a.ensureParent(rel)
	if nil != err {
		return nil, err
	}
	tmp, err := a.writeTemp(ctx, dirRel, newOverwriteName(path.Base(filepath.ToSlash(rel))), reader, "failed to create overwrite file")
	if nil != err {
		return nil, err
	}
	tmp.target = rel
	return &Overwrite{adapter: a, tmp: tmp, dirRel: dirRel, rel: rel}, nil
}

func (o *Overwrite) TempPath() string {
	return o.tmp.rel
}

func (o *Overwrite) OldPath() string {
	return o.oldRel
}

func (o *Overwrite) Discard() {
	o.tmp.discard(o.adapter)
}

func (o *Overwrite) Swap(oldETag string) error {
	a := o.adapter
	oldRel := path.Join(o.dirRel, overwriteName(path.Base(filepath.ToSlash(o.rel)), overwriteOldMarker, oldETag))
	if err := a.root.Rename(o.rel, oldRel); nil == err {
		o.oldRel = oldRel
	} else if !os.IsNotExist(err) {
		o.Discard()
		return fmt.Errorf("failed to set aside current file: %w", err)
	}
	if err := a.root.Rename(o.tmp.rel, o.rel); nil != err {
		o.Discard()
		cause := fmt.Errorf("failed to move new file into place: %w", err)
		if "" == o.oldRel {
			return cause
		}
		if restoreErr := a.root.Rename(o.oldRel, o.rel); nil != restoreErr {
			return errors.Join(cause, fmt.Errorf("failed to restore current file: %w", restoreErr))
		}
		o.oldRel = ""
		return errors.Join(cause, a.syncer.sync(o.dirRel))
	}
	if err := a.settle(o.tmp, o.dirRel); nil != err {
		cause := fmt.Errorf("failed to flush overwrite: %w", err)
		if restoreErr := o.Restore(); nil != restoreErr {
			return errors.Join(cause, restoreErr)
		}
		return cause
	}
	return nil
}

func (o *Overwrite) Restore() error {
	a := o.adapter
	if "" == o.oldRel {
		if err := a.root.Remove(o.rel); nil != err && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove new file %s: %w", o.rel, err)
		}
	} else if err := a.root.Rename(o.oldRel, o.rel); nil != err {
		return fmt.Errorf("failed to restore previous file %s: %w", o.rel, err)
	}
	o.oldRel = ""
	if nil != a.flusher {
		a.flusher.forget(o.rel)
	}
	if err := a.syncer.sync(o.dirRel); nil != err {
		return fmt.Errorf("failed to flush restored file %s: %w", o.rel, err)
	}
	return nil
}

func (o *Overwrite) DropOld() error {
	if "" == o.oldRel {
		return nil
	}
	if err := o.adapter.root.Remove(o.oldRel); nil != err && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove previous file copy %s: %w", o.oldRel, err)
	}
	return nil
}

func (a *LocalFSAdapter) OverwriteLeftovers(ctx context.Context, skip ...string) ([]OverwriteLeftover, error) {
	var found []OverwriteLeftover
	err := fs.WalkDir(a.root.FS(), ".", func(p string, entry fs.DirEntry, err error) error {
		if nil != err {
			return err
		}
		if ctxErr := ctx.Err(); nil != ctxErr {
			return ctxErr
		}
		if entry.IsDir() {
			if slices.Contains(skip, p) {
				return fs.SkipDir
			}
			return nil
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		target, tag, old, ok := parseOverwriteName(entry.Name())
		if !ok {
			return nil
		}
		leftover := OverwriteLeftover{Path: p, Target: path.Join(path.Dir(p), target), Old: old}
		if old {
			leftover.ETag = tag
		}
		found = append(found, leftover)
		return nil
	})
	if nil != err {
		return nil, fmt.Errorf("failed to scan for overwrite leftovers: %w", err)
	}
	return found, nil
}

func (a *LocalFSAdapter) RestoreLeftover(leftover OverwriteLeftover) error {
	if err := a.root.Rename(leftover.Path, leftover.Target); nil != err {
		return fmt.Errorf("failed to restore %s from %s: %w", leftover.Target, leftover.Path, err)
	}
	if nil != a.flusher {
		a.flusher.forget(leftover.Target)
	}
	return a.syncer.sync(path.Dir(leftover.Path))
}

func (a *LocalFSAdapter) RemoveLeftover(leftover OverwriteLeftover) error {
	if err := a.root.Remove(leftover.Path); nil != err && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove overwrite leftover %s: %w", leftover.Path, err)
	}
	return nil
}

func (a *LocalFSAdapter) purgeOverwriteLeftovers(rel string) bool {
	entries, err := fs.ReadDir(a.root.FS(), rel)
	if nil != err || 0 == len(entries) {
		return false
	}
	for _, entry := range entries {
		if !entry.Type().IsRegular() || !IsOverwriteName(entry.Name()) {
			return false
		}
	}
	for _, entry := range entries {
		if err := a.root.Remove(path.Join(rel, entry.Name())); nil != err && !os.IsNotExist(err) {
			return false
		}
	}
	return true
}
