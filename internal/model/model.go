package model

import "time"

type Result struct {
	URL     string
	Status  int
	Err     error
	Elapsed time.Duration
}

func (r Result) IsDead() bool {
	if r.Err != nil {
		return true
	}

	return r.Status >= 400
}
