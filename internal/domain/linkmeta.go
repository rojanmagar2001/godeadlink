package domain

type LinkMeta struct {
	URL            string
	FirstSeenDepth int
	Sources        map[string]struct{}
	Kind           LinkKind
	Skipped        SkipReason // optional; for skipped counting
}
