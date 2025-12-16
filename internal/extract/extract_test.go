package extract

import (
	"strings"
	"testing"

	"github.com/rojanmagar2001/godeadlink/internal/model"
)

func TestExtractLinks_ExtractsPagesAndAssets(t *testing.T) {
	html := `
	<html><head>
		<link href="/style.css" rel="stylesheet">
	</head>
	<body>
		<a href="/page">page</a>
		<img src="/img.png">
		<script src="/app.js"></script>
	</body></html>`

	found, err := ExtractLinks("https://example.com/base/", strings.NewReader(html))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Collect checkable URLs by kind
	got := map[string]model.LinkKind{}
	for _, f := range found {
		if f.SkipReason == "" {
			got[f.URL] = f.Kind
		}
	}

	want := map[string]model.LinkKind{
		"https://example.com/page":      model.LinkKindPage,
		"https://example.com/style.css": model.LinkKindAsset,
		"https://example.com/img.png":   model.LinkKindAsset,
		"https://example.com/app.js":    model.LinkKindAsset,
	}

	if len(got) != len(want) {
		t.Fatalf("got %d, want %d: %#v", len(got), len(want), got)
	}
	for u, k := range want {
		if got[u] != k {
			t.Fatalf("expected %s kind %s, got %s", u, k, got[u])
		}
	}
}

func TestExtractLinks_SkipsFragmentAndUnsupportedSchemes(t *testing.T) {
	html := `
	<html><body>
		<a href="#section">frag</a>
		<a href="mailto:test@example.com">mail</a>
		<a href="tel:+123">tel</a>
		<a href="javascript:void(0)">js</a>
	</body></html>`

	found, err := ExtractLinks("https://example.com", strings.NewReader(html))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var frag, unsup int
	for _, f := range found {
		switch f.SkipReason {
		case model.SkipFragmentOnly:
			frag++
		case model.SkipUnsupportedScheme:
			unsup++
		}
	}

	if frag != 1 {
		t.Fatalf("expected 1 fragment skip, got %d", frag)
	}
	if unsup != 3 {
		t.Fatalf("expected 3 unsupported scheme skips, got %d", unsup)
	}
}
