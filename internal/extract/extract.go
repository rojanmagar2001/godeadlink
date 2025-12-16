package extract

import (
	"fmt"
	"io"
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

// ExtractLinks  finds <a href="..."> values, resolves them against baseURL,
// skips empty and non-http(s) schemes, removes fragments for uniqueness.
func ExtractLinks(baseURL string, r io.Reader) ([]string, error) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse base url: %w", err)
	}

	doc, err := html.Parse(r)
	if err != nil {
		return nil, fmt.Errorf("parse html: %w", err)
	}

	seen := make(map[string]struct{})
	var out []string

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, a := range n.Attr {
				if strings.EqualFold(a.Key, "href") {
					href := strings.TrimSpace(a.Val)
					if href == "" {
						continue
					}

					u, err := url.Parse(href)
					if err != nil {
						continue // ignore malformed hrefs in stage 1
					}

					resolved := base.ResolveReference(u)

					// Skip non-http(s)
					switch strings.ToLower(resolved.Scheme) {
					case "http", "https":
					default:
						continue
					}

					// Drop fragment for uniqueness
					resolved.Fragment = ""

					s := resolved.String()
					if _, ok := seen[s]; ok {
						continue
					}
					seen[s] = struct{}{}
					out = append(out, s)
				}
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}

	walk(doc)

	return out, nil
}
