package ports

import (
	"io"

	"github.com/rojanmagar2001/godeadlink/internal/domain"
)

type Extractor interface {
	Extract(baseUrl string, r io.Reader) ([]domain.FoundLink, error)
}
