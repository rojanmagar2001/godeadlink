package check

import (
	"net/http"
	"time"
)

type Checker struct {
	Client      *http.Client
	HeadFirst   bool
	MaxBodyRead int64
}

func NewChecker(timeout time.Duration, headFirst bool) *Checker {
	return &Checker{
		Client: &http.Client{
			Timeout: timeout,
		},
		HeadFirst:   headFirst,
		MaxBodyRead: 1 << 20, // 1MB safety cap
	}
}
