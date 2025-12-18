package extractor

import (
	"io"

	"github.com/rojanmagar2001/godeadlink/internal/domain"
	"github.com/rojanmagar2001/godeadlink/internal/extract"
)

type Adapter struct{}

func New() *Adapter { return &Adapter{} }

func (a *Adapter) Extract(baseURL string, r io.Reader) ([]domain.FoundLink, error) {
	found, err := extract.ExtractLinks(baseURL, r)
	if err != nil {
		return nil, err
	}

	out := make([]domain.FoundLink, 0, len(found))
	for _, f := range found {
		out = append(out, domain.FoundLink{
			URL:        f.URL,
			Kind:       domain.LinkKind(f.Kind),
			SkipReason: domain.SkipReason(f.SkipReason),
			Raw:        f.Raw,
		})
	}

	return out, nil
}
