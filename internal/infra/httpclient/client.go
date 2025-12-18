package httpclient

import (
	"net/http"
	"time"
)

type Client struct {
	c *http.Client
}

func New(timeout time.Duration) *Client {
	return &Client{c: &http.Client{Timeout: timeout}}
}

func (c *Client) Do(req *http.Request) (*http.Response, error) {
	return c.c.Do(req)
}

func (c *Client) Timeout() float64 { return 0 }
