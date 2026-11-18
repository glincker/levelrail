// Package trafficclass sorts each edge request into one class so analytics can
// tell page views from API calls, assets and bots. It is a pure function of the
// request and response headers: no state, no allocation on the hot path.
package trafficclass

import (
	"net/http"
	"strings"
)

// Class is the kind of request the edge saw. The zero value is Other.
type Class uint8

// The six analytics classes, then Ignored for requests a path rule excludes.
const (
	Other Class = iota
	Page
	API
	Asset
	Bot
	Prefetch
	// Ignored counts in RED metrics but never in analytics (health checks).
	Ignored
)

// Count is the number of user-visible classes (Ignored is not one of them).
const Count = int(Ignored)

var classNames = [...]string{"other", "page", "api", "asset", "bot", "prefetch", "ignored"}

// String is the wire name of the class.
func (c Class) String() string {
	if int(c) < len(classNames) {
		return classNames[c]
	}
	return classNames[Other]
}

// Response is what the classifier needs from the answer.
type Response struct {
	Status      int
	ContentType string
	// Shielded is true when the abuse shield already turned the request away.
	Shielded bool
}

// Result is the class plus the facts a caller may record next to it.
type Result struct {
	Class Class
	// Soft is a client-side navigation (a Next.js RSC fetch), counted as a page view.
	Soft bool
	// Bot names the matched crawler token, or UnknownBot.
	Bot string
	// Rule is the 1-based index of the path rule that decided, 0 when none did.
	Rule int
}

// UnknownBot labels an automated client that matched no named token.
const UnknownBot = "unknown automated"

// Classify decides the class of one finished request. The first matching step
// wins: path rules, bots, prefetch, client navigation, Sec-Fetch-Dest, fallback.
// A nil rules value means DefaultRules.
func Classify(r *http.Request, resp Response, rules *Rules) Result {
	if rules == nil {
		rules = DefaultRules()
	}
	if idx, c, ok := rules.match(r.URL.Path); ok {
		return Result{Class: c, Rule: idx + 1}
	}
	if name, ok := botName(r.UserAgent(), resp.Shielded); ok {
		return Result{Class: Bot, Bot: name}
	}
	h := r.Header
	if isPrefetch(h) {
		return Result{Class: Prefetch}
	}
	// Checked before Sec-Fetch-Dest: a Next.js navigation is a fetch (dest empty).
	if h.Get("Rsc") == "1" {
		return Result{Class: Page, Soft: true}
	}
	if dest := h.Get("Sec-Fetch-Dest"); dest != "" {
		return Result{Class: fromFetchDest(dest)}
	}
	return Result{Class: fallback(r, resp)}
}

// IsPageView reports whether a page-class request counts as a page view: a
// successful GET. Redirects and error documents are not views.
func IsPageView(method string, status int) bool {
	return method == http.MethodGet && status >= 200 && status < 300
}

func isPrefetch(h http.Header) bool {
	return containsFold(h.Get("Sec-Purpose"), "prefetch") ||
		containsFold(h.Get("Purpose"), "prefetch") ||
		strings.EqualFold(h.Get("X-Moz"), "prefetch") ||
		h.Get("Next-Router-Prefetch") != ""
}

func fromFetchDest(dest string) Class {
	switch strings.ToLower(dest) {
	case "document":
		return Page
	case "empty":
		return API
	case "iframe", "frame", "embed", "object", "script", "style", "image", "font", "manifest",
		"audio", "video", "track", "worker", "sharedworker", "serviceworker", "report", "xslt":
		return Asset
	}
	return Other
}

// assetExts are the extensions the no-header fallback treats as static files.
var assetExts = []string{
	".js", ".css", ".png", ".jpg", ".jpeg", ".gif", ".svg", ".webp", ".avif", ".ico", ".woff", ".woff2",
	".ttf", ".map", ".mp4", ".webm", ".txt", ".xml",
}

func fallback(r *http.Request, resp Response) Class {
	ct, accept := resp.ContentType, r.Header.Get("Accept")
	if r.Method == http.MethodGet && containsFold(ct, "text/html") && containsFold(accept, "text/html") {
		return Page
	}
	if isJSONish(ct) || isJSONish(accept) || containsFold(ct, "text/event-stream") ||
		containsFold(accept, "text/event-stream") || strings.EqualFold(r.Header.Get("X-Requested-With"), "XMLHttpRequest") {
		return API
	}
	for _, ext := range assetExts {
		if hasSuffixFold(r.URL.Path, ext) {
			return Asset
		}
	}
	return Other
}

func isJSONish(v string) bool {
	return containsFold(v, "application/json") || containsFold(v, "+json")
}

// containsFold reports whether s contains the lowercase ASCII token sub, ignoring case, without allocating.
func containsFold(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if equalFoldAt(s[i:i+len(sub)], sub) {
			return true
		}
	}
	return false
}

func hasSuffixFold(s, suffix string) bool {
	return len(s) >= len(suffix) && equalFoldAt(s[len(s)-len(suffix):], suffix)
}

// equalFoldAt compares s to the lowercase ASCII string lower.
func equalFoldAt(s, lower string) bool {
	for i := 0; i < len(lower); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		if c != lower[i] {
			return false
		}
	}
	return true
}
