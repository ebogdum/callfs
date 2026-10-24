package fsm

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ebogdum/callfs/v2/metadata"
)

func (m *Machine) GetErasureInfo(filePath string) (*metadata.ErasureFileInfo, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	info, ok := m.state.ErasureByPath[filePath]
	if !ok {
		return nil, false
	}
	return CloneErasureFileInfo(info), true
}

func (m *Machine) ListErasureInfos(prefix, cursor string, limit int) ([]*metadata.ErasureFileInfo, string, error) {
	if 0 >= limit || MaxPageLimit < limit {
		return nil, "", fmt.Errorf("%w: limit must be between 1 and %d", metadata.ErrInvalidQuery, MaxPageLimit)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	paths := make([]string, 0, len(m.state.ErasureByPath))
	for path := range m.state.ErasureByPath {
		if strings.HasPrefix(path, prefix) && path > cursor {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	next := ""
	if limit < len(paths) {
		paths = paths[:limit]
		next = paths[limit-1]
	}
	infos := make([]*metadata.ErasureFileInfo, 0, len(paths))
	for _, path := range paths {
		infos = append(infos, CloneErasureFileInfo(m.state.ErasureByPath[path]))
	}
	return infos, next, nil
}
