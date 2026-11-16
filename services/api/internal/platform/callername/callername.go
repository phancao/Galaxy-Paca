// Package callername gives Paca API's self-declared name for calls to Vortex
// Identity /internal/* (SPA-8542). Identity logs one [internal-caller] line per
// /internal/* call and reads the caller from the X-Service-Name header.
package callername

import (
	"net/http"
	"os"
	"regexp"
	"strings"
)

// Header is the request header Identity reads the caller name from.
const Header = "X-Service-Name"

// DefaultServiceName is used when GALAXY_SERVICE_NAME is unset or blank.
const DefaultServiceName = "paca-api"

const maxLen = 64

var unsafeChars = regexp.MustCompile(`[^A-Za-z0-9._:@/+=-]`)

// ServiceName returns GALAXY_SERVICE_NAME (read at call time, trimmed) or
// DefaultServiceName, with characters outside [A-Za-z0-9._:@/+=-] replaced by
// "-" and cut to 64 characters.
func ServiceName() string {
	name := strings.TrimSpace(os.Getenv("GALAXY_SERVICE_NAME"))
	if name == "" {
		name = DefaultServiceName
	}
	name = unsafeChars.ReplaceAllString(name, "-")
	if len(name) > maxLen {
		name = name[:maxLen]
	}
	return name
}

// Set adds X-Service-Name to h. Self-declared, for logs only — not proof of
// identity; anyone holding X-Service-Secret can send any name.
func Set(h http.Header) {
	h.Set(Header, ServiceName())
}
