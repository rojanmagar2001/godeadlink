package ports

import "context"

type Limiter interface {
	Take(ctx context.Context, rawURL string) error
}
