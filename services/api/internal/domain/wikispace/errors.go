package wikispacedom

import "errors"

// Sentinel domain errors for the wikispace aggregate.
var (
	ErrSpaceNotFound = errors.New("wikispace: project has no wiki space")
	ErrLinkNotFound  = errors.New("wikispace: task wiki link not found")
	// ErrDisabled indicates the Wiki integration is not configured
	// (missing WIKI_API_URL / WIKI_API_TOKEN).
	ErrDisabled = errors.New("wikispace: wiki integration disabled")
)
