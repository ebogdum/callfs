package fsm

import (
	"container/heap"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ebogdum/callfs/v2/metadata"
)

const MaxPageLimit = 1000

var linkStatusFilters = map[string]struct{}{
	"":        {},
	"active":  {},
	"used":    {},
	"expired": {},
	"revoked": {},
}

type expiryEntry struct {
	at    time.Time
	token string
}

type expiryHeap []expiryEntry

func (h expiryHeap) Len() int           { return len(h) }
func (h expiryHeap) Less(i, j int) bool { return h[i].at.Before(h[j].at) }
func (h expiryHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *expiryHeap) Push(x any) {
	*h = append(*h, x.(expiryEntry))
}

func (h *expiryHeap) Pop() any {
	old := *h
	last := old[len(old)-1]
	*h = old[:len(old)-1]
	return last
}

type linkCursor struct {
	createdNanos int64
	id           int64
}

func LinkIDForToken(token string) int64 {
	sum := sha256.Sum256([]byte(token))
	return int64(binary.BigEndian.Uint64(sum[:8]) >> 1)
}

func effectiveLinkStatus(link *metadata.SingleUseLink, now time.Time) string {
	if "active" == link.Status && link.ExpiresAt.Before(now) {
		return "expired"
	}
	return link.Status
}

func linkOrder(a, b metadata.SingleUseLink) bool {
	createdA, createdB := a.CreatedAt.UnixNano(), b.CreatedAt.UnixNano()
	if createdA != createdB {
		return createdA < createdB
	}
	return a.ID < b.ID
}

func linkAfterCursor(link metadata.SingleUseLink, after linkCursor) bool {
	created := link.CreatedAt.UnixNano()
	return created > after.createdNanos || (created == after.createdNanos && link.ID > after.id)
}

func encodeLinkCursor(link metadata.SingleUseLink) string {
	raw := strconv.FormatInt(link.CreatedAt.UnixNano(), 10) + ":" + strconv.FormatInt(link.ID, 10)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeLinkCursor(cursor string) (linkCursor, bool, error) {
	if "" == cursor {
		return linkCursor{}, false, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if nil != err {
		return linkCursor{}, false, fmt.Errorf("%w: malformed cursor", metadata.ErrInvalidQuery)
	}
	createdPart, idPart, found := strings.Cut(string(raw), ":")
	if !found {
		return linkCursor{}, false, fmt.Errorf("%w: malformed cursor", metadata.ErrInvalidQuery)
	}
	created, errCreated := strconv.ParseInt(createdPart, 10, 64)
	id, errID := strconv.ParseInt(idPart, 10, 64)
	if nil != errCreated || nil != errID {
		return linkCursor{}, false, fmt.Errorf("%w: malformed cursor", metadata.ErrInvalidQuery)
	}
	return linkCursor{createdNanos: created, id: id}, true, nil
}

func encodeOwnerCursor(owner string) string {
	return base64.RawURLEncoding.EncodeToString([]byte("o" + owner))
}

func decodeOwnerCursor(cursor string) (string, bool, error) {
	if "" == cursor {
		return "", false, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if nil != err || 0 == len(raw) || 'o' != raw[0] {
		return "", false, fmt.Errorf("%w: malformed cursor", metadata.ErrInvalidQuery)
	}
	return string(raw[1:]), true, nil
}

func (m *Machine) indexLink(token string, link *metadata.SingleUseLink) {
	switch {
	case "active" == link.Status:
		heap.Push(&m.activeLinkExpiry, expiryEntry{at: link.ExpiresAt, token: token})
	case "used" == link.Status && nil != link.UsedAt:
		heap.Push(&m.usedLinkTime, expiryEntry{at: *link.UsedAt, token: token})
	}
}

func (m *Machine) rebuildLinkIndex() {
	m.activeLinkExpiry = nil
	m.usedLinkTime = nil
	m.linkIDs = make(map[int64][]string, len(m.state.LinksByToken))
	for token, link := range m.state.LinksByToken {
		m.indexLink(token, link)
		m.indexLinkID(token)
	}
}

func (m *Machine) indexLinkID(token string) {
	id := LinkIDForToken(token)
	m.linkIDs[id] = append(m.linkIDs[id], token)
}

func (m *Machine) unindexLinkID(token string) {
	id := LinkIDForToken(token)
	tokens := m.linkIDs[id]
	for i, candidate := range tokens {
		if candidate == token {
			tokens = append(tokens[:i], tokens[i+1:]...)
			break
		}
	}
	if 0 == len(tokens) {
		delete(m.linkIDs, id)
		return
	}
	m.linkIDs[id] = tokens
}

func (m *Machine) drainLinks(h *expiryHeap, status string, cutoff time.Time, due func(link *metadata.SingleUseLink) bool) int {
	count := 0
	for 0 < h.Len() && (*h)[0].at.Before(cutoff) {
		entry := heap.Pop(h).(expiryEntry)
		link, ok := m.state.LinksByToken[entry.token]
		if !ok || status != link.Status || !due(link) {
			continue
		}
		delete(m.state.LinksByToken, entry.token)
		m.unindexLinkID(entry.token)
		count++
	}
	return count
}

func (m *Machine) tokenForLinkID(id int64) (string, bool) {
	found := ""
	ok := false
	for _, token := range m.linkIDs[id] {
		if _, exists := m.state.LinksByToken[token]; !exists {
			continue
		}
		if !ok || token < found {
			found = token
			ok = true
		}
	}
	return found, ok
}

func (m *Machine) linkByID(id int64, now time.Time) (*metadata.SingleUseLink, bool) {
	token, ok := m.tokenForLinkID(id)
	if !ok {
		return nil, false
	}
	link := CloneLink(m.state.LinksByToken[token])
	link.ID = id
	link.Status = effectiveLinkStatus(link, now)
	return link, true
}

func (m *Machine) linksMatching(filter metadata.LinkFilter, now time.Time) []metadata.SingleUseLink {
	out := make([]metadata.SingleUseLink, 0)
	for token, stored := range m.state.LinksByToken {
		status := effectiveLinkStatus(stored, now)
		if "" != filter.Status && filter.Status != status {
			continue
		}
		if "" != filter.FilePath && filter.FilePath != stored.FilePath {
			continue
		}
		if "" != filter.CreatedBy && filter.CreatedBy != stored.Owner {
			continue
		}
		link := CloneLink(stored)
		link.ID = LinkIDForToken(token)
		link.Status = status
		out = append(out, *link)
	}
	sort.Slice(out, func(i, j int) bool { return linkOrder(out[i], out[j]) })
	return out
}

func (m *Machine) ownerTotals() map[string]metadata.OwnerStat {
	totals := make(map[string]metadata.OwnerStat)
	for _, md := range m.state.MetadataByPath {
		if "file" != md.Type {
			continue
		}
		stat := totals[md.Owner]
		stat.Owner = md.Owner
		stat.Files++
		stat.Bytes += md.Size
		totals[md.Owner] = stat
	}
	return totals
}

func (m *Machine) revokeLinkByID(cmd *Command) Result {
	token, ok := m.tokenForLinkID(cmd.LinkID)
	if !ok {
		return Result{Err: "not_found"}
	}
	link := m.state.LinksByToken[token]
	if "active" != link.Status {
		return Result{Err: "precondition_failed"}
	}
	updated := CloneLink(link)
	updated.Status = "revoked"
	if !cmd.Timestamp.IsZero() {
		updated.UpdatedAt = cmd.Timestamp
	}
	m.state.LinksByToken[token] = updated
	return Result{}
}

func (m *Machine) GetLink(token string) (*metadata.SingleUseLink, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	link, ok := m.state.LinksByToken[token]
	if !ok {
		return nil, false
	}
	return CloneLink(link), true
}

func (m *Machine) LinkByID(id int64, now time.Time) (*metadata.SingleUseLink, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.linkByID(id, now)
}

func (m *Machine) CountActiveLinks(owner string, now time.Time) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	count := 0
	for _, link := range m.state.LinksByToken {
		if owner == link.Owner && "active" == link.Status && now.Before(link.ExpiresAt) {
			count++
		}
	}
	return count
}

func (m *Machine) ListLinks(filter metadata.LinkFilter) ([]metadata.SingleUseLink, string, error) {
	if 0 >= filter.Limit || MaxPageLimit < filter.Limit {
		return nil, "", fmt.Errorf("%w: limit must be between 1 and %d", metadata.ErrInvalidQuery, MaxPageLimit)
	}
	if _, known := linkStatusFilters[filter.Status]; !known {
		return nil, "", fmt.Errorf("%w: unknown link status %q", metadata.ErrInvalidQuery, filter.Status)
	}
	after, hasAfter, err := decodeLinkCursor(filter.Cursor)
	if nil != err {
		return nil, "", err
	}
	now := filter.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	m.mu.RLock()
	links := m.linksMatching(filter, now)
	m.mu.RUnlock()

	page := make([]metadata.SingleUseLink, 0, filter.Limit)
	next := ""
	for _, link := range links {
		if hasAfter && !linkAfterCursor(link, after) {
			continue
		}
		if len(page) == filter.Limit {
			next = encodeLinkCursor(page[len(page)-1])
			break
		}
		page = append(page, link)
	}
	return page, next, nil
}

func (m *Machine) OwnerStats(cursor string, limit int) ([]metadata.OwnerStat, string, error) {
	if 0 >= limit || MaxPageLimit < limit {
		return nil, "", fmt.Errorf("%w: limit must be between 1 and %d", metadata.ErrInvalidQuery, MaxPageLimit)
	}
	after, hasAfter, err := decodeOwnerCursor(cursor)
	if nil != err {
		return nil, "", err
	}
	m.mu.RLock()
	totals := m.ownerTotals()
	m.mu.RUnlock()

	owners := make([]string, 0, len(totals))
	for owner := range totals {
		owners = append(owners, owner)
	}
	sort.Strings(owners)

	page := make([]metadata.OwnerStat, 0, limit)
	next := ""
	for _, owner := range owners {
		if hasAfter && owner <= after {
			continue
		}
		if len(page) == limit {
			next = encodeOwnerCursor(page[len(page)-1].Owner)
			break
		}
		page = append(page, totals[owner])
	}
	return page, next, nil
}
