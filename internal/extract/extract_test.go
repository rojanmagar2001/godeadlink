package extract

import (
	"strings"
	"testing"
)

func TestExtractLinks_BasicResolution(t *testing.T) {
	html := `
	<html><body>
		<a href="/a">A</a>
		<a href="b">B</a>
	</body></html>
	`

	links, err := ExtractLinks("https://example.com/base/", strings.NewReader(html))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := map[string]bool{
		"https://example.com/a":      true,
		"https://example.com/base/b": true,
	}

	if len(links) != len(want) {
		t.Fatalf("got %d links, want %d", len(links), len(want))
	}

	for _, l := range links {
		if !want[l] {
			t.Fatalf("unexpected link: %s", l)
		}
	}
}

func TestExtractLinks_SkipsUnsupportedSchemes(t *testing.T) {
	html := `
	<html><body>
		<a href="mailto:test@example.com">mail</a>
		<a href="tel:+123">tel</a>
		<a href="javascript:void(0)">js</a>
	</body></html>`

	links, err := ExtractLinks("https://example.com", strings.NewReader(html))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(links) != 0 {
		t.Fatalf("expected 0 links, got %d: %#v", len(links), links)
	}
}

func TestExtractLinks_DeduplicatesFragments(t *testing.T) {
	html := `
	<html><body>
		<a href="/page#one">one</a>
		<a href="/page#two">two</a>
	</body></html>`

	links, err := ExtractLinks("https://example.com", strings.NewReader(html))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(links) != 1 {
		t.Fatalf("expected 1 link, got %d: %#v", len(links), links)
	}

	if links[0] != "https://example.com/page" {
		t.Fatalf("unexpected link: %s", links[0])
	}
}

func TestExtractLinks_ResolvesAndDedupes(t *testing.T) {
	html := `
	<html><body>
		<a href="/ok">OK</a>
		<a href="/ok#frag">OK2</a>
		<a href="https://example.com/abs">ABS</a>
		<a href="mailto:test@example.com">MAIL</a>
		<a href="">EMPTY</a>
	</body></html>`

	links, err := ExtractLinks("http://localhost:1234/base", strings.NewReader(html))
	if err != nil {
		t.Fatalf("ExtractLinks error: %v", err)
	}

	// Expect:
	// - http://localhost:1234/ok   (fragment dropped, deduped)
	// - https://example.com/abs
	want := map[string]bool{
		"http://localhost:1234/ok": true,
		"https://example.com/abs":  true,
	}

	if len(links) != len(want) {
		t.Fatalf("got %d links, want %d: %#v", len(links), len(want), links)
	}
	for _, got := range links {
		if !want[got] {
			t.Fatalf("unexpected link: %s", got)
		}
	}
}
