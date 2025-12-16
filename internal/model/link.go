package model

type LinkKind string

const (
	LinkKindPage  LinkKind = "page"
	LinkKindAsset LinkKind = "asset"
)

type SkipReason string

const (
	SkipFragmentOnly      SkipReason = "fragment_only"
	SkipUnsupportedScheme SkipReason = "unsupported_scheme"
	SkipInvalidURL        SkipReason = "invalid_url"
	SkipExternal          SkipReason = "external"
	SkipEmpty             SkipReason = "empty"
)
