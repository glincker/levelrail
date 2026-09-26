package preview

import (
	"errors"
	"fmt"
	"io"
	"net/netip"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"

	"github.com/GLINCKER/levelrail/internal/netguard"
)

const (
	maxTitleLen = 200
	maxDescLen  = 300
	maxURLLen   = 2048
)

var themeColorPattern = regexp.MustCompile(`^#(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)

// PageMeta is what the metadata tier reads from a page's head. URL fields are
// raw and unresolved; use ResolveRef before fetching any of them.
type PageMeta struct {
	Title        string
	OGTitle      string
	Description  string
	ThemeColor   string
	OGImage      string
	TwitterImage string
	Icon         string
}

// ParseMeta tokenizes HTML from r and collects the head metadata it knows.
// It stops at the first body element or when r is exhausted.
func ParseMeta(r io.Reader) PageMeta {
	var m PageMeta
	z := html.NewTokenizer(r)
	inTitle := false
	var iconRank int
	for {
		switch z.Next() {
		case html.ErrorToken:
			return m.clean()
		case html.TextToken:
			if inTitle && m.Title == "" {
				m.Title = strings.TrimSpace(string(z.Text()))
			}
		case html.EndTagToken:
			name, _ := z.TagName()
			if string(name) == "title" {
				inTitle = false
			}
		case html.StartTagToken, html.SelfClosingTagToken:
			name, hasAttr := z.TagName()
			switch string(name) {
			case "body":
				return m.clean()
			case "title":
				inTitle = true
			case "meta":
				if hasAttr {
					m.readMeta(attrs(z))
				}
			case "link":
				if hasAttr {
					m.readLink(attrs(z), &iconRank)
				}
			}
		}
	}
}

func attrs(z *html.Tokenizer) map[string]string {
	out := map[string]string{}
	for {
		k, v, more := z.TagAttr()
		if key := strings.ToLower(string(k)); key != "" {
			if _, dup := out[key]; !dup {
				out[key] = string(v)
			}
		}
		if !more {
			return out
		}
	}
}

func (m *PageMeta) readMeta(a map[string]string) {
	key := strings.ToLower(strings.TrimSpace(a["property"]))
	if key == "" {
		key = strings.ToLower(strings.TrimSpace(a["name"]))
	}
	content := strings.TrimSpace(a["content"])
	if content == "" {
		return
	}
	set := func(dst *string) {
		if *dst == "" {
			*dst = content
		}
	}
	switch key {
	case "og:image", "og:image:url", "og:image:secure_url":
		set(&m.OGImage)
	case "twitter:image", "twitter:image:src":
		set(&m.TwitterImage)
	case "og:title":
		set(&m.OGTitle)
	case "og:description", "description":
		if key == "og:description" || m.Description == "" {
			m.Description = content
		}
	case "theme-color":
		set(&m.ThemeColor)
	}
}

func (m *PageMeta) readLink(a map[string]string, rank *int) {
	href := strings.TrimSpace(a["href"])
	if href == "" {
		return
	}
	for _, rel := range strings.Fields(strings.ToLower(a["rel"])) {
		r := 0
		switch rel {
		case "icon":
			r = 1
		case "apple-touch-icon":
			r = 2
		}
		if r > *rank {
			*rank = r
			m.Icon = href
		}
	}
}

func (m PageMeta) clean() PageMeta {
	m.Title = truncate(collapse(m.Title), maxTitleLen)
	m.OGTitle = truncate(collapse(m.OGTitle), maxTitleLen)
	m.Description = truncate(collapse(m.Description), maxDescLen)
	if !themeColorPattern.MatchString(m.ThemeColor) {
		m.ThemeColor = ""
	}
	return m
}

func collapse(s string) string { return strings.Join(strings.Fields(s), " ") }

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// DisplayTitle prefers og:title over the document title.
func (m PageMeta) DisplayTitle() string {
	if m.OGTitle != "" {
		return m.OGTitle
	}
	return m.Title
}

// ImageRef is the raw image reference to try: og:image first, then twitter:image.
func (m PageMeta) ImageRef() string {
	if m.OGImage != "" {
		return m.OGImage
	}
	return m.TwitterImage
}

// ErrBadRef means a reference is not a fetchable http(s) URL.
var ErrBadRef = errors.New("preview: unusable reference")

// ResolveRef resolves raw against base and accepts only absolute http(s) URLs
// without credentials. A literal internal IP is refused here; DNS names that
// resolve internally are refused by the guarded dialer.
func ResolveRef(base *url.URL, raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > maxURLLen {
		return nil, fmt.Errorf("%w: empty or too long", ErrBadRef)
	}
	ref, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadRef, err)
	}
	u := base.ResolveReference(ref)
	if (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil {
		return nil, fmt.Errorf("%w: scheme %q", ErrBadRef, u.Scheme)
	}
	if ip, perr := netip.ParseAddr(u.Hostname()); perr == nil && netguard.IsBlocked(ip) && !sameHost(u, base) {
		return nil, fmt.Errorf("%w: internal address", ErrBadRef)
	}
	return u, nil
}

func sameHost(a, b *url.URL) bool {
	return strings.EqualFold(a.Host, b.Host)
}
