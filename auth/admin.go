package auth

import "sync"

const rootUserID = "root"

type AdminSet struct {
	mu  sync.RWMutex
	ids map[string]struct{}
}

func NewAdminSet(ids []string) *AdminSet {
	return &AdminSet{ids: toIDSet(ids)}
}

func (s *AdminSet) IsAdmin(id string) bool {
	if rootUserID == id {
		return true
	}
	if nil == s {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.ids[id]
	return ok
}

func (s *AdminSet) Replace(ids []string) {
	set := toIDSet(ids)
	s.mu.Lock()
	s.ids = set
	s.mu.Unlock()
}

func toIDSet(ids []string) map[string]struct{} {
	set := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if "" != id {
			set[id] = struct{}{}
		}
	}
	return set
}
