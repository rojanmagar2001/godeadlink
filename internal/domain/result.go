package domain

import "time"

type Result struct {
	URL        string
	StatusCode int
	Err        error
	Elapsed    time.Duration
}

func (r Result) IsDead() bool {
	if r.Err != nil {
		return true
	}

	return r.StatusCode >= 400
}
