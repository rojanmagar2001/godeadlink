package store

import (
	"net/url"
	"sort"
	"strings"
	"sync"

	"github.com/rojanmagar2001/godeadlink/internal/domain"
)

type Memory struct {
	mu sync.Mutex

	visited map[string]struct{}
	links   map[string]*domain.LinkMeta
}

func NewMemory() *Memory {
	return &Memory{
		visited: make(map[string]struct{}),
		links:   make(map[string]*domain.LinkMeta),
	}
}

func (m *Memory) MarkVisitedPage(url string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	k := normalizeForKey(url)
	if _, ok := m.visited[k]; ok {
		return false
	}
	m.visited[k] = struct{}{}
	return true
}

func (m *Memory) VisitedCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.visited)
}

func (m *Memory) RecordDiscoveredLink(meta domain.LinkMeta, sourcePage string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	k := normalizeForKey(meta.URL)
	ex, ok := m.links[k]
	if !ok {
		meta.URL = k
		if meta.Sources == nil {
			meta.Sources = map[string]struct{}{}
		}
		m.links[k] = &meta
		ex = &meta
	}

	if meta.FirstSeenDepth < ex.FirstSeenDepth {
		ex.FirstSeenDepth = meta.FirstSeenDepth
	}
	if meta.Kind != "" {
		ex.Kind = meta.Kind
	}
	if meta.Skipped != "" {
		ex.Skipped = meta.Skipped
	}

	if sourcePage != "" {
		ex.Sources[normalizeForKey(sourcePage)] = struct{}{}
	}
}

func (m *Memory) AllDiscovered() []*domain.LinkMeta {
	m.mu.Lock()
	defer m.mu.Unlock()

	out := make([]*domain.LinkMeta, 0, len(m.links))
	for _, v := range m.links {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].URL < out[j].URL })
	return out
}

// normalizeForKey is a small normalization to improve deduping:
// - strip fragment
// - lowercase hostname
// (More robust normalization will come later.)
func normalizeForKey(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	u.Fragment = ""
	if u.Host != "" {
		// url.URL doesn't have Hostname setter, so normalize via Host field.
		// Keep port if present.
		host := strings.ToLower(u.Hostname())
		if port := u.Port(); port != "" {
			u.Host = host + ":" + port
		} else {
			u.Host = host
		}
	}
	return u.String()
}
