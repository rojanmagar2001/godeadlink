package ports

import "net/http"

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
	Timeout() (seconds float64) // optional hook (can return 0)
}
