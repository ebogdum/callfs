package fsm

import (
	"strings"
	"time"

	"github.com/ebogdum/callfs/v2/metadata"
)

const (
	MaxBatchParts  = 1000
	StagedBatchTTL = 30 * time.Second
)

type stagedBatch struct {
	FirstAt time.Time `json:"first_at"`
	Next    int       `json:"next"`
	Ops     []Command `json:"ops"`
}

func (m *Machine) expireStagedBatches(now time.Time) {
	if now.IsZero() || 0 == len(m.state.StagedBatches) {
		return
	}
	for id, staged := range m.state.StagedBatches {
		if StagedBatchTTL < now.Sub(staged.FirstAt) {
			delete(m.state.StagedBatches, id)
		}
	}
}

func (m *Machine) applyBatchPart(cmd *Command) Result {
	if "" == cmd.BatchID {
		return Result{Err: ErrCodeBatchIDRequired}
	}
	staged, exists := m.state.StagedBatches[cmd.BatchID]
	if !exists {
		if 0 != cmd.BatchSeq {
			return Result{Err: ErrCodeBatchIncomplete}
		}
		staged = &stagedBatch{FirstAt: cmd.Timestamp}
	}
	if cmd.BatchSeq != staged.Next {
		delete(m.state.StagedBatches, cmd.BatchID)
		return Result{Err: ErrCodeBatchOutOfOrder}
	}
	if MaxBatchParts <= staged.Next {
		delete(m.state.StagedBatches, cmd.BatchID)
		return Result{Err: ErrCodeBatchTooLarge}
	}
	staged.Ops = append(staged.Ops, cmd.Ops...)
	staged.Next++
	if !cmd.BatchCommit {
		m.state.StagedBatches[cmd.BatchID] = staged
		return Result{}
	}
	delete(m.state.StagedBatches, cmd.BatchID)
	assembled := Command{Op: "apply_batch", Ops: staged.Ops, Timestamp: cmd.Timestamp}
	return m.applyBatch(&assembled)
}

func (m *Machine) applyBatch(cmd *Command) Result {
	view := newBatchView(m)
	for i := range cmd.Ops {
		if res := view.check(&cmd.Ops[i]); "" != res.Err {
			return res
		}
	}
	for i := range cmd.Ops {
		sub := cmd.Ops[i]
		sub.Timestamp = cmd.Timestamp
		sub.inBatch = true
		var res Result
		if "erasure" == opCategory[sub.Op] {
			res = m.applyErasureOp(&sub)
		} else {
			res = m.applyMetadataOp(&sub)
		}
		if "" != res.Err {
			return res
		}
	}
	return Result{}
}

type batchView struct {
	m        *Machine
	metadata map[string]*metadata.Metadata
	erasure  map[string]bool
}

func newBatchView(m *Machine) *batchView {
	return &batchView{
		m:        m,
		metadata: map[string]*metadata.Metadata{},
		erasure:  map[string]bool{},
	}
}

func (v *batchView) lookup(path string) (*metadata.Metadata, bool) {
	if md, ok := v.metadata[path]; ok {
		return md, nil != md
	}
	md, ok := v.m.state.MetadataByPath[path]
	return md, ok
}

func (v *batchView) hasErasure(path string) bool {
	if present, ok := v.erasure[path]; ok {
		return present
	}
	_, ok := v.m.state.ErasureByPath[path]
	return ok
}

func (v *batchView) check(cmd *Command) Result {
	switch cmd.Op {
	case "create_metadata":
		if nil == cmd.Metadata {
			return Result{Err: "metadata_required"}
		}
		if _, exists := v.lookup(cmd.Metadata.Path); exists {
			return Result{Err: "already_exists"}
		}
		if code := parentProblem(cmd.Metadata.Path, v.lookup); "" != code {
			return Result{Err: code}
		}
		v.metadata[cmd.Metadata.Path] = cmd.Metadata
	case "update_metadata":
		if nil == cmd.Metadata {
			return Result{Err: "metadata_required"}
		}
		if _, exists := v.lookup(cmd.Metadata.Path); !exists {
			return Result{Err: "not_found"}
		}
		v.metadata[cmd.Metadata.Path] = cmd.Metadata
	case "update_metadata_if_match":
		if nil == cmd.Metadata {
			return Result{Err: "metadata_required"}
		}
		existing, exists := v.lookup(cmd.Metadata.Path)
		if !exists {
			return Result{Err: "not_found"}
		}
		if existing.ETag != cmd.ExpectedETag {
			return Result{Err: "precondition_failed"}
		}
		v.metadata[cmd.Metadata.Path] = cmd.Metadata
	case "delete_metadata":
		if _, exists := v.lookup(cmd.Path); !exists {
			return Result{Err: "not_found"}
		}
		v.metadata[cmd.Path] = nil
		v.erasure[cmd.Path] = false
	case "rename_metadata":
		src, exists := v.lookup(cmd.Path)
		if !exists {
			return Result{Err: "not_found"}
		}
		if _, exists := v.lookup(cmd.NewPath); exists {
			return Result{Err: "already_exists"}
		}
		if code := parentProblem(cmd.NewPath, v.lookup); "" != code {
			return Result{Err: code}
		}
		v.rename(src, cmd.Path, cmd.NewPath)
	case "create_erasure_info":
		if nil == cmd.ErasureInfo {
			return Result{Err: "erasure_info_required"}
		}
		if v.hasErasure(cmd.Path) {
			return Result{Err: "already_exists"}
		}
		v.erasure[cmd.Path] = true
	case "delete_erasure_info":
		v.erasure[cmd.Path] = false
	default:
		return Result{Err: ErrCodeUnknownOperation}
	}
	return Result{}
}

func (v *batchView) rename(src *metadata.Metadata, oldPath, newPath string) {
	isDir := "directory" == src.Type
	affected := []string{oldPath}
	if isDir && "/" != oldPath {
		affected = append(affected, v.descendants(oldPath)...)
	}
	moved := make([]*metadata.Metadata, len(affected))
	for i, oldP := range affected {
		moved[i], _ = v.lookup(oldP)
	}
	for _, oldP := range affected {
		v.metadata[oldP] = nil
	}
	for i, oldP := range affected {
		target := newPath + oldP[len(oldPath):]
		clone := CloneMetadata(moved[i])
		clone.Path = target
		clone.Name = pathBase(target)
		v.metadata[target] = clone
	}

	erasureAffected := make([]string, 0)
	matches := func(p string) bool {
		return p == oldPath || (isDir && strings.HasPrefix(p, oldPath+"/"))
	}
	for p := range v.m.state.ErasureByPath {
		if matches(p) && v.hasErasure(p) {
			erasureAffected = append(erasureAffected, p)
		}
	}
	for p, present := range v.erasure {
		if present && matches(p) {
			if _, inBase := v.m.state.ErasureByPath[p]; !inBase {
				erasureAffected = append(erasureAffected, p)
			}
		}
	}
	for _, oldP := range erasureAffected {
		v.erasure[oldP] = false
	}
	for _, oldP := range erasureAffected {
		v.erasure[newPath+oldP[len(oldPath):]] = true
	}
}

func (v *batchView) descendants(dirPath string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0)
	for _, p := range v.m.appendDescendants(nil, dirPath) {
		if _, exists := v.lookup(p); exists {
			seen[p] = struct{}{}
			out = append(out, p)
		}
	}
	prefix := dirPath + "/"
	for p, md := range v.metadata {
		if nil == md || !strings.HasPrefix(p, prefix) {
			continue
		}
		if _, dup := seen[p]; dup {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}
