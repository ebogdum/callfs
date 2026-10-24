package fsm

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ebogdum/callfs/v2/metadata"
)

const Version = 1

const (
	ErrCodeNotFound           = "not_found"
	ErrCodeAlreadyExists      = "already_exists"
	ErrCodePrecondition       = "precondition_failed"
	ErrCodeFenced             = "fenced"
	ErrCodeParentMissing      = "parent_missing"
	ErrCodeParentNotDirectory = "parent_not_directory"
	ErrCodeDirectoryNotEmpty  = "directory_not_empty"
	ErrCodeUnknownOperation   = "unknown_operation"
	ErrCodeUnsupportedVersion = "unsupported_version"
	ErrCodeInvalidCommand     = "invalid_command"
	ErrCodeBatchIDRequired    = "batch_id_required"
	ErrCodeBatchSequence      = "batch_sequence_invalid"
	ErrCodeBatchIncomplete    = "batch_incomplete"
	ErrCodeBatchOutOfOrder    = "batch_out_of_order"
	ErrCodeBatchTooLarge      = "batch_too_large"
)

var (
	ErrFenced             = errors.New("fencing token is older than the highest token seen")
	ErrUnsupportedCommand = errors.New("command is not supported by this metadata machine version")
)

type Fence struct {
	Key   string `json:"key"`
	Token uint64 `json:"token"`
}

type Command struct {
	LinkID       int64                     `json:"link_id,omitempty"`
	Op           string                    `json:"op"`
	Path         string                    `json:"path,omitempty"`
	NewPath      string                    `json:"new_path,omitempty"`
	Metadata     *metadata.Metadata        `json:"metadata,omitempty"`
	ExpectedETag string                    `json:"expected_etag,omitempty"`
	Prefix       string                    `json:"prefix,omitempty"`
	StartAfter   string                    `json:"start_after,omitempty"`
	Limit        int                       `json:"limit,omitempty"`
	MaxDepth     int                       `json:"max_depth,omitempty"`
	Token        string                    `json:"token,omitempty"`
	Link         *metadata.SingleUseLink   `json:"link,omitempty"`
	Status       string                    `json:"status,omitempty"`
	UsedAt       *time.Time                `json:"used_at,omitempty"`
	UsedByIP     *string                   `json:"used_by_ip,omitempty"`
	Before       *time.Time                `json:"before,omitempty"`
	OlderThan    *time.Time                `json:"older_than,omitempty"`
	ErasureInfo  *metadata.ErasureFileInfo `json:"erasure_info,omitempty"`
	Ops          []Command                 `json:"ops,omitempty"`
	Timestamp    time.Time                 `json:"timestamp,omitempty"`
	V            int                       `json:"v,omitempty"`
	Fence        *Fence                    `json:"fence,omitempty"`
	BatchID      string                    `json:"batch_id,omitempty"`
	BatchSeq     int                       `json:"batch_seq,omitempty"`
	BatchCommit  bool                      `json:"batch_commit,omitempty"`
	Origin       string                    `json:"origin,omitempty"`
	Nonce        string                    `json:"nonce,omitempty"`

	inBatch bool
}

type Result struct {
	CleanupCount int                     `json:"cleanup_count,omitempty"`
	Err          string                  `json:"err,omitempty"`
	Consumed     bool                    `json:"consumed,omitempty"`
	Metadata     *metadata.Metadata      `json:"metadata,omitempty"`
	Link         *metadata.SingleUseLink `json:"link,omitempty"`
	Children     []*metadata.Metadata    `json:"children,omitempty"`
	Index        uint64                  `json:"index,omitempty"`
	Halt         bool                    `json:"halt,omitempty"`
	Results      []Result                `json:"results,omitempty"`
}

func (r Result) AsError() error {
	if r.Halt {
		return fmt.Errorf("%w: %s", ErrUnsupportedCommand, r.Err)
	}
	switch r.Err {
	case "":
		return nil
	case ErrCodeNotFound:
		return metadata.ErrNotFound
	case ErrCodeAlreadyExists:
		return metadata.ErrAlreadyExists
	case ErrCodePrecondition:
		return metadata.ErrPrecondition
	case ErrCodeFenced:
		return ErrFenced
	case ErrCodeParentMissing:
		return ErrParentMissing
	case ErrCodeParentNotDirectory:
		return ErrParentNotDirectory
	case ErrCodeDirectoryNotEmpty:
		return ErrDirectoryNotEmpty
	}
	return errors.New(r.Err)
}

type state struct {
	MetadataByPath map[string]*metadata.Metadata        `json:"metadata_by_path"`
	LinksByToken   map[string]*metadata.SingleUseLink   `json:"links_by_token"`
	ErasureByPath  map[string]*metadata.ErasureFileInfo `json:"erasure_by_path"`
	LastAt         time.Time                            `json:"last_at,omitzero"`
	Fences         map[string]uint64                    `json:"fences,omitempty"`
	StagedBatches  map[string]*stagedBatch              `json:"staged_batches,omitempty"`
}

func newState() state {
	return state{
		MetadataByPath: map[string]*metadata.Metadata{},
		LinksByToken:   map[string]*metadata.SingleUseLink{},
		ErasureByPath:  map[string]*metadata.ErasureFileInfo{},
		Fences:         map[string]uint64{},
		StagedBatches:  map[string]*stagedBatch{},
	}
}

func (s *state) fillMissing() {
	if nil == s.MetadataByPath {
		s.MetadataByPath = map[string]*metadata.Metadata{}
	}
	if nil == s.LinksByToken {
		s.LinksByToken = map[string]*metadata.SingleUseLink{}
	}
	if nil == s.ErasureByPath {
		s.ErasureByPath = map[string]*metadata.ErasureFileInfo{}
	}
	if nil == s.Fences {
		s.Fences = map[string]uint64{}
	}
	if nil == s.StagedBatches {
		s.StagedBatches = map[string]*stagedBatch{}
	}
}

type Machine struct {
	mu    sync.RWMutex
	state state

	childIndex map[string]map[string]struct{}

	linkIDs map[int64][]string

	activeLinkExpiry expiryHeap
	usedLinkTime     expiryHeap
}

func New() *Machine {
	return &Machine{
		state:      newState(),
		childIndex: map[string]map[string]struct{}{},
		linkIDs:    map[int64][]string{},
	}
}

var opCategory = map[string]string{
	"revoke_link":              "link",
	"get_metadata":             "metadata",
	"list_children":            "metadata",
	"count_children":           "metadata",
	"create_metadata":          "metadata",
	"update_metadata":          "metadata",
	"update_metadata_if_match": "metadata",
	"delete_metadata":          "metadata",
	"rename_metadata":          "metadata",
	"get_link":                 "link",
	"create_link":              "link",
	"update_link":              "link",
	"consume_link":             "link",
	"cleanup_expired_links":    "link",
	"cleanup_used_links":       "link",
	"create_erasure_info":      "erasure",
	"delete_erasure_info":      "erasure",
	"apply_batch":              "batch",
	"group":                    "group",
	"batch_part":               "batch_part",
	"noop":                     "noop",
}

func (m *Machine) ApplyBytes(data []byte, at time.Time) Result {
	var cmd Command
	if err := json.Unmarshal(data, &cmd); nil != err {
		return Result{Err: fmt.Sprintf("%s:%v", ErrCodeInvalidCommand, err), Halt: true}
	}
	return m.Apply(cmd, at)
}

func (m *Machine) Apply(cmd Command, at time.Time) Result {
	if res := unsupported(&cmd); res.Halt {
		return res
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if !at.IsZero() {
		at = at.UTC()
		if at.Before(m.state.LastAt) {
			at = m.state.LastAt
		}
		m.state.LastAt = at
		cmd.Timestamp = at
	}
	m.expireStagedBatches(cmd.Timestamp)
	if "group" == opCategory[cmd.Op] {
		results := make([]Result, len(cmd.Ops))
		for i := range cmd.Ops {
			grouped := cmd.Ops[i]
			grouped.Timestamp = cmd.Timestamp
			if res := unsupported(&grouped); res.Halt {
				results[i] = res
				continue
			}
			results[i] = m.applyLocked(&grouped)
		}
		return Result{Results: results}
	}
	return m.applyLocked(&cmd)
}

func (m *Machine) applyLocked(cmdPtr *Command) Result {
	cmd := *cmdPtr
	if nil != cmd.Fence {
		if res := m.checkFence(cmd.Fence); "" != res.Err {
			return res
		}
	}

	switch opCategory[cmd.Op] {
	case "metadata":
		return m.applyMetadataOp(&cmd)
	case "link":
		return m.applyLinkOp(&cmd)
	case "erasure":
		return m.applyErasureOp(&cmd)
	case "batch":
		return m.applyBatch(&cmd)
	case "batch_part":
		return m.applyBatchPart(&cmd)
	case "noop":
		return Result{}
	}
	return Result{Err: ErrCodeUnknownOperation, Halt: true}
}

func unsupported(cmd *Command) Result {
	if Version < cmd.V {
		return Result{Err: fmt.Sprintf("%s:%d", ErrCodeUnsupportedVersion, cmd.V), Halt: true}
	}
	if _, known := opCategory[cmd.Op]; !known {
		return Result{Err: ErrCodeUnknownOperation, Halt: true}
	}
	for i := range cmd.Ops {
		if _, known := opCategory[cmd.Ops[i].Op]; !known {
			return Result{Err: ErrCodeUnknownOperation, Halt: true}
		}
	}
	return Result{}
}

func (m *Machine) checkFence(fence *Fence) Result {
	if current, seen := m.state.Fences[fence.Key]; seen && fence.Token < current {
		return Result{Err: ErrCodeFenced}
	}
	m.state.Fences[fence.Key] = fence.Token
	return Result{}
}

func (m *Machine) applyMetadataOp(cmd *Command) Result {
	switch cmd.Op {
	case "get_metadata":
		md, ok := m.state.MetadataByPath[cmd.Path]
		if !ok {
			return Result{Err: "not_found"}
		}
		return Result{Metadata: CloneMetadata(md)}
	case "create_metadata":
		if nil == cmd.Metadata {
			return Result{Err: "metadata_required"}
		}
		if _, exists := m.state.MetadataByPath[cmd.Metadata.Path]; exists {
			return Result{Err: "already_exists"}
		}
		if code := parentProblem(cmd.Metadata.Path, m.lookup); "" != code {
			return Result{Err: code}
		}
		m.state.MetadataByPath[cmd.Metadata.Path] = CloneMetadata(cmd.Metadata)
		m.indexChild(cmd.Metadata.Path)
		return Result{}
	case "update_metadata":
		if nil == cmd.Metadata {
			return Result{Err: "metadata_required"}
		}
		if _, exists := m.state.MetadataByPath[cmd.Metadata.Path]; !exists {
			return Result{Err: "not_found"}
		}
		m.state.MetadataByPath[cmd.Metadata.Path] = CloneMetadata(cmd.Metadata)
		return Result{}
	case "update_metadata_if_match":
		if nil == cmd.Metadata {
			return Result{Err: "metadata_required"}
		}
		existing, exists := m.state.MetadataByPath[cmd.Metadata.Path]
		if !exists {
			return Result{Err: "not_found"}
		}
		if existing.ETag != cmd.ExpectedETag {
			return Result{Err: "precondition_failed"}
		}
		m.state.MetadataByPath[cmd.Metadata.Path] = CloneMetadata(cmd.Metadata)
		return Result{}
	case "delete_metadata":
		existing, exists := m.state.MetadataByPath[cmd.Path]
		if !exists {
			return Result{Err: "not_found"}
		}
		if !cmd.inBatch && "directory" == existing.Type && 0 < m.countChildren(cmd.Path) {
			return Result{Err: ErrCodeDirectoryNotEmpty}
		}
		delete(m.state.MetadataByPath, cmd.Path)
		delete(m.state.ErasureByPath, cmd.Path)
		m.unindexChild(cmd.Path)
		return Result{}
	case "rename_metadata":
		return m.applyRename(cmd)
	default:
		return Result{Children: m.childrenOf(cmd.Path)}
	}
}

func (m *Machine) applyRename(cmd *Command) Result {
	src, exists := m.state.MetadataByPath[cmd.Path]
	if !exists {
		return Result{Err: "not_found"}
	}
	if _, exists := m.state.MetadataByPath[cmd.NewPath]; exists {
		return Result{Err: "already_exists"}
	}
	if code := parentProblem(cmd.NewPath, m.lookup); "" != code {
		return Result{Err: code}
	}
	now := cmd.Timestamp
	affected := []string{cmd.Path}
	if "directory" == src.Type && "/" != cmd.Path {
		affected = m.appendDescendants(affected, cmd.Path)
	}
	erasureAffected := []string{}
	for p := range m.state.ErasureByPath {
		if p == cmd.Path || ("directory" == src.Type && strings.HasPrefix(p, cmd.Path+"/")) {
			erasureAffected = append(erasureAffected, p)
		}
	}
	for _, oldP := range affected {
		moved := CloneMetadata(m.state.MetadataByPath[oldP])
		newP := cmd.NewPath + oldP[len(cmd.Path):]
		moved.Path = newP
		moved.Name = pathBase(newP)
		if !now.IsZero() {
			moved.UpdatedAt = now
		}
		delete(m.state.MetadataByPath, oldP)
		m.unindexChild(oldP)
		m.state.MetadataByPath[newP] = moved
		m.indexChild(newP)
	}
	for _, oldP := range erasureAffected {
		info := m.state.ErasureByPath[oldP]
		delete(m.state.ErasureByPath, oldP)
		m.state.ErasureByPath[cmd.NewPath+oldP[len(cmd.Path):]] = info
	}
	return Result{}
}

func (m *Machine) applyLinkOp(cmd *Command) Result {
	switch cmd.Op {
	case "revoke_link":
		return m.revokeLinkByID(cmd)
	case "get_link":
		link, ok := m.state.LinksByToken[cmd.Token]
		if !ok {
			return Result{Err: "not_found"}
		}
		return Result{Link: CloneLink(link)}
	case "create_link":
		if nil == cmd.Link {
			return Result{Err: "link_required"}
		}
		if _, exists := m.state.LinksByToken[cmd.Link.Token]; exists {
			return Result{Err: "already_exists"}
		}
		stored := CloneLink(cmd.Link)
		m.state.LinksByToken[cmd.Link.Token] = stored
		m.indexLink(cmd.Link.Token, stored)
		m.indexLinkID(cmd.Link.Token)
		return Result{}
	case "update_link":
		link, exists := m.state.LinksByToken[cmd.Token]
		if !exists || "active" != link.Status {
			return Result{Err: "not_found"}
		}
		updated := CloneLink(link)
		updated.Status = cmd.Status
		updated.UsedAt = cloneTimePtr(cmd.UsedAt)
		updated.UsedByIP = cloneStringPtr(cmd.UsedByIP)
		if !cmd.Timestamp.IsZero() {
			updated.UpdatedAt = cmd.Timestamp
		}
		m.state.LinksByToken[cmd.Token] = updated
		m.indexLink(cmd.Token, updated)
		return Result{}
	case "consume_link":
		return m.consumeLink(cmd)
	case "cleanup_expired_links":
		if nil == cmd.Before {
			return Result{Err: "before_required"}
		}
		cutoff := *cmd.Before
		return Result{CleanupCount: m.drainLinks(&m.activeLinkExpiry, "active", cutoff, func(link *metadata.SingleUseLink) bool {
			return link.ExpiresAt.Before(cutoff)
		})}
	default:
		if nil == cmd.OlderThan {
			return Result{Err: "older_than_required"}
		}
		cutoff := *cmd.OlderThan
		return Result{CleanupCount: m.drainLinks(&m.usedLinkTime, "used", cutoff, func(link *metadata.SingleUseLink) bool {
			return nil != link.UsedAt && link.UsedAt.Before(cutoff)
		})}
	}
}

func (m *Machine) consumeLink(cmd *Command) Result {
	if nil == cmd.UsedAt {
		return Result{Err: "used_at_required"}
	}
	link, exists := m.state.LinksByToken[cmd.Token]
	if !exists {
		return Result{Err: "not_found"}
	}
	now := *cmd.UsedAt
	if "active" != link.Status || !now.Before(link.ExpiresAt) {
		return Result{Link: CloneLink(link)}
	}
	updated := CloneLink(link)
	updated.Status = "used"
	updated.UsedAt = cloneTimePtr(cmd.UsedAt)
	updated.UsedByIP = cloneStringPtr(cmd.UsedByIP)
	updated.UpdatedAt = now
	m.state.LinksByToken[cmd.Token] = updated
	m.indexLink(cmd.Token, updated)
	return Result{Link: CloneLink(updated), Consumed: true}
}

func (m *Machine) applyErasureOp(cmd *Command) Result {
	if "create_erasure_info" == cmd.Op {
		if nil == cmd.ErasureInfo {
			return Result{Err: "erasure_info_required"}
		}
		if _, exists := m.state.ErasureByPath[cmd.Path]; exists {
			return Result{Err: "already_exists"}
		}
		m.state.ErasureByPath[cmd.Path] = CloneErasureFileInfo(cmd.ErasureInfo)
		return Result{}
	}
	delete(m.state.ErasureByPath, cmd.Path)
	return Result{}
}

var (
	ErrParentMissing      = fmt.Errorf("%w: parent directory does not exist", metadata.ErrNotFound)
	ErrParentNotDirectory = fmt.Errorf("%w: parent is not a directory", metadata.ErrPrecondition)
	ErrDirectoryNotEmpty  = fmt.Errorf("%w: directory is not empty", metadata.ErrPrecondition)
)

func (m *Machine) lookup(path string) (*metadata.Metadata, bool) {
	md, ok := m.state.MetadataByPath[path]
	return md, ok
}

func parentProblem(path string, lookup func(string) (*metadata.Metadata, bool)) string {
	parent := pathDir(path)
	if "/" == parent || parent == path {
		return ""
	}
	md, ok := lookup(parent)
	if !ok || nil == md {
		return ErrCodeParentMissing
	}
	if "directory" != md.Type {
		return ErrCodeParentNotDirectory
	}
	return ""
}
