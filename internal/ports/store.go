package ports

import "github.com/rojanmagar2001/godeadlink/internal/domain"

// Store is in-memory for now. Later we can swap for sqlite/bolt.
type Store interface {
	MarkVisitedPage(url string) bool // returns true if it was newly marked
	VisitedCount() int

	RecordDiscoveredLink(linkURL domain.LinkMeta, sourcePage string)
	AllDiscovered() []*domain.LinkMeta
}
