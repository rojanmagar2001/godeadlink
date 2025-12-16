package extract

import (
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/rojanmagar2001/godeadlink/internal/model"
	"golang.org/x/net/html"
)

type FoundLink struct {
	URL        string
	Kind       model.LinkKind
	SkipReason model.SkipReason
	Raw        string
}

// ExtractLinks  finds <a href="..."> values, resolves them against baseURL,
// skips empty and non-http(s) schemes, removes fragments for uniqueness.
func ExtractLinks(baseURL string, r io.Reader) ([]FoundLink, error) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse base url: %w", err)
	}

	doc, err := html.Parse(r)
	if err != nil {
		return nil, fmt.Errorf("parse html: %w", err)
	}

	seen := make(map[string]struct{})
	var out []FoundLink

	emit := func(raw string, resolved *url.URL, kind model.LinkKind, skip model.SkipReason) {
		var final string
		if resolved != nil {
			// Drop fragment for uniqueness of “real” URLs
			resolved.Fragment = ""
			final = resolved.String()
		}

		// Dedup rule:
		// - for checkable links: dedup by the resolved final URL
		// - for skipped links: dedup by (reason + kind + raw) so different
		//   unsupported schemes don't collapse into one
		key := final
		if skip != "" {
			key = fmt.Sprintf("%s|%s|%s", skip, kind, raw)
		}
		if key == "" {
			// extremely defensive fallback
			key = fmt.Sprintf("empty|%s|%s", kind, raw)
		}

		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}

		out = append(out, FoundLink{
			URL:        final,
			Kind:       kind,
			SkipReason: skip,
			Raw:        raw,
		})
	}

	isUnsupportedScheme := func(scheme string) bool {
		switch strings.ToLower(scheme) {
		case "http", "https", "":
			return false
		default:
			return true
		}
	}

	// Etract helper for specific tag/attribute combos
	extractAttr := func(n *html.Node, tag, attr string, kind model.LinkKind) {
		if n.Type != html.ElementNode || n.Data != tag {
			return
		}

		for _, a := range n.Attr {
			if strings.EqualFold(a.Key, attr) {
				raw := strings.TrimSpace(a.Val)

				if raw == "" {
					emit(raw, nil, kind, model.SkipEmpty)
				}
				if strings.HasPrefix(raw, "#") {
					emit(raw, nil, kind, model.SkipFragmentOnly)
				}

				parsed, err := url.Parse(raw)
				if err != nil {
					// Keep as “invalid” by returning a pseudo skipped item via unsupported scheme? No:
					// We’ll treat invalid URLs as checkable later by emitting an empty SkipReason but raw string.
					// For stage 4 we keep it simple: mark unsupported_scheme to indicate “not checkable”.
					emit(raw, nil, kind, model.SkipInvalidURL)
					return
				}

				// Resolve relative references
				resolved := base.ResolveReference(parsed)

				// Skip unsupported schemes like mailto/tel/javascript/data
				if isUnsupportedScheme(resolved.Scheme) {
					emit(raw, nil, kind, model.SkipUnsupportedScheme)
					return
				}

				emit(raw, resolved, kind, "")
				return
			}
		}
	}

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		// Pages
		extractAttr(n, "a", "href", model.LinkKindPage)

		// Assets
		extractAttr(n, "img", "src", model.LinkKindAsset)
		extractAttr(n, "script", "src", model.LinkKindAsset)
		extractAttr(n, "link", "href", model.LinkKindAsset)

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}

	walk(doc)

	return out, nil
}
