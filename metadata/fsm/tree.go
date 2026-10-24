package fsm

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/ebogdum/callfs/v2/metadata"
)

func (m *Machine) indexChild(path string) {
	if "/" == path {
		return
	}
	parent := pathDir(path)
	if nil == m.childIndex[parent] {
		m.childIndex[parent] = make(map[string]struct{})
	}
	m.childIndex[parent][path] = struct{}{}
}

func (m *Machine) unindexChild(path string) {
	if "/" == path {
		return
	}
	parent := pathDir(path)
	if set := m.childIndex[parent]; nil != set {
		delete(set, path)
		if 0 == len(set) {
			delete(m.childIndex, parent)
		}
	}
}

func (m *Machine) rebuildChildIndex() {
	m.childIndex = make(map[string]map[string]struct{}, len(m.state.MetadataByPath))
	for path := range m.state.MetadataByPath {
		m.indexChild(path)
	}
}

func (m *Machine) childrenOf(parentPath string) []*metadata.Metadata {
	children := make([]*metadata.Metadata, 0, len(m.childIndex[parentPath]))
	for path := range m.childIndex[parentPath] {
		if md, ok := m.state.MetadataByPath[path]; ok {
			children = append(children, CloneMetadata(md))
		}
	}
	return children
}

func (m *Machine) countChildren(parentPath string) int {
	count := 0
	for path := range m.childIndex[parentPath] {
		if _, ok := m.state.MetadataByPath[path]; ok {
			count++
		}
	}
	return count
}

func (m *Machine) appendDescendants(paths []string, root string) []string {
	stack := []string{root}
	for 0 < len(stack) {
		dir := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for child := range m.childIndex[dir] {
			paths = append(paths, child)
			stack = append(stack, child)
		}
	}
	return paths
}

func (m *Machine) filesByPrefix(prefix, startAfter string, limit int) []*metadata.Metadata {
	paths := make([]string, 0)
	for path, md := range m.state.MetadataByPath {
		if "file" == md.Type && strings.HasPrefix(path, prefix) && path > startAfter {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	if 0 > limit {
		limit = 0
	}
	if limit < len(paths) {
		paths = paths[:limit]
	}
	out := make([]*metadata.Metadata, 0, len(paths))
	for _, path := range paths {
		out = append(out, CloneMetadata(m.state.MetadataByPath[path]))
	}
	return out
}

type descendantFrame struct {
	path  string
	depth int
}

func (m *Machine) descendantsOf(dirPath string, maxDepth, limit int) []*metadata.Metadata {
	stack := make([]descendantFrame, 0, len(m.childIndex[dirPath]))
	for child := range m.childIndex[dirPath] {
		stack = append(stack, descendantFrame{path: child, depth: 1})
	}
	out := make([]*metadata.Metadata, 0, len(stack))
	for 0 < len(stack) {
		frame := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		md, ok := m.state.MetadataByPath[frame.path]
		if !ok {
			continue
		}
		out = append(out, md)
		if 0 != maxDepth && frame.depth >= maxDepth {
			continue
		}
		for child := range m.childIndex[frame.path] {
			stack = append(stack, descendantFrame{path: child, depth: frame.depth + 1})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	if 0 < limit && limit < len(out) {
		out = out[:limit]
	}
	for i, md := range out {
		out[i] = CloneMetadata(md)
	}
	return out
}

func (m *Machine) searchDescendants(dirPath string, maxDepth, limit int, match func(*metadata.Metadata) bool) ([]*metadata.Metadata, int) {
	stack := make([]descendantFrame, 0, len(m.childIndex[dirPath]))
	for child := range m.childIndex[dirPath] {
		stack = append(stack, descendantFrame{path: child, depth: 1})
	}
	paths := make([]string, 0, len(stack))
	for 0 < len(stack) {
		frame := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if _, ok := m.state.MetadataByPath[frame.path]; !ok {
			continue
		}
		paths = append(paths, frame.path)
		if 0 != maxDepth && frame.depth >= maxDepth {
			continue
		}
		for child := range m.childIndex[frame.path] {
			stack = append(stack, descendantFrame{path: child, depth: frame.depth + 1})
		}
	}
	sort.Strings(paths)
	if 0 < limit && limit < len(paths) {
		paths = paths[:limit]
	}
	matched := make([]*metadata.Metadata, 0)
	for _, p := range paths {
		md := m.state.MetadataByPath[p]
		if match(md) {
			matched = append(matched, CloneMetadata(md))
		}
	}
	return matched, len(paths)
}

func (m *Machine) Get(path string) (*metadata.Metadata, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	md, ok := m.state.MetadataByPath[path]
	if !ok {
		return nil, false
	}
	return CloneMetadata(md), true
}

func (m *Machine) ListChildren(parentPath string) []*metadata.Metadata {
	m.mu.RLock()
	children := m.childrenOf(parentPath)
	m.mu.RUnlock()
	sort.Slice(children, func(i, j int) bool { return children[i].Path < children[j].Path })
	return children
}

func (m *Machine) HasChildren(parentPath string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return 0 < len(m.childIndex[parentPath])
}

func (m *Machine) CountChildren(parentPath string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.countChildren(parentPath)
}

func (m *Machine) ListDescendants(dirPath string, maxDepth, limit int) []*metadata.Metadata {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.descendantsOf(dirPath, maxDepth, limit)
}

func (m *Machine) SearchDescendants(dirPath string, maxDepth, limit int, match func(*metadata.Metadata) bool) ([]*metadata.Metadata, int) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.searchDescendants(dirPath, maxDepth, limit, match)
}

func (m *Machine) ListByPrefix(prefix, startAfter string, limit int) []*metadata.Metadata {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.filesByPrefix(prefix, startAfter, limit)
}

func (m *Machine) CountEmptyETags() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	count := 0
	for _, md := range m.state.MetadataByPath {
		if "file" == md.Type && "" == md.ETag {
			count++
		}
	}
	return count
}

func (m *Machine) CountEmptyOwners() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	count := 0
	for _, md := range m.state.MetadataByPath {
		if "" == md.Owner {
			count++
		}
	}
	return count
}

type StateCounts struct {
	Files          int
	Directories    int
	LinksActive    int
	LinksUsed      int
	LinksExpired   int
	LinksRevoked   int
	ErasureEntries int
	IndexEntries   int
}

func (m *Machine) StateCounts(now time.Time) StateCounts {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var counts StateCounts
	for _, md := range m.state.MetadataByPath {
		if "directory" == md.Type {
			counts.Directories++
		} else {
			counts.Files++
		}
	}
	for _, link := range m.state.LinksByToken {
		switch effectiveLinkStatus(link, now) {
		case "active":
			counts.LinksActive++
		case "used":
			counts.LinksUsed++
		case "expired":
			counts.LinksExpired++
		case "revoked":
			counts.LinksRevoked++
		}
	}
	counts.ErasureEntries = len(m.state.ErasureByPath)
	for _, children := range m.childIndex {
		counts.IndexEntries += len(children)
	}
	return counts
}

type searchView struct {
	metadata.Store
	machine *Machine
}

func (v searchView) ListChildren(ctx context.Context, parentPath string) ([]*metadata.Metadata, error) {
	return v.machine.ListChildren(parentPath), nil
}

func (v searchView) ListByPrefix(ctx context.Context, prefix, startAfter string, limit int) ([]*metadata.Metadata, error) {
	return v.machine.ListByPrefix(prefix, startAfter, limit), nil
}

func (m *Machine) Search(ctx context.Context, q metadata.SearchQuery) (metadata.SearchResult, error) {
	return metadata.SearchByScan(ctx, searchView{machine: m}, q)
}

func CloneMetadata(in *metadata.Metadata) *metadata.Metadata {
	if nil == in {
		return nil
	}
	block := &metadataClone{md: *in}
	if nil != in.ParentID {
		block.parentID = *in.ParentID
		block.md.ParentID = &block.parentID
	}
	if nil != in.CallFSInstanceID {
		block.instanceID = *in.CallFSInstanceID
		block.md.CallFSInstanceID = &block.instanceID
	}
	if nil != in.SymlinkTarget {
		block.symlinkTarget = *in.SymlinkTarget
		block.md.SymlinkTarget = &block.symlinkTarget
	}
	return &block.md
}

type metadataClone struct {
	md            metadata.Metadata
	parentID      int64
	instanceID    string
	symlinkTarget string
}

func CloneLink(in *metadata.SingleUseLink) *metadata.SingleUseLink {
	if nil == in {
		return nil
	}
	out := *in
	out.UsedAt = cloneTimePtr(in.UsedAt)
	out.UsedByIP = cloneStringPtr(in.UsedByIP)
	return &out
}

func CloneErasureFileInfo(in *metadata.ErasureFileInfo) *metadata.ErasureFileInfo {
	if nil == in {
		return nil
	}
	out := *in
	if nil != in.Shards {
		out.Shards = make([]metadata.ErasureShardInfo, len(in.Shards))
		copy(out.Shards, in.Shards)
	}
	return &out
}

func cloneStringPtr(in *string) *string {
	if nil == in {
		return nil
	}
	v := *in
	return &v
}

func cloneTimePtr(in *time.Time) *time.Time {
	if nil == in {
		return nil
	}
	v := *in
	return &v
}

func pathDir(p string) string {
	if "/" == p {
		return "/"
	}
	lastSlash := strings.LastIndex(p, "/")
	if 0 >= lastSlash {
		return "/"
	}
	return p[:lastSlash]
}

func pathBase(p string) string {
	if idx := strings.LastIndex(p, "/"); 0 <= idx {
		return p[idx+1:]
	}
	return p
}
