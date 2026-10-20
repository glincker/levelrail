package preview

import (
	"net/url"
	"strings"
	"testing"
)

func TestParseMeta(t *testing.T) {
	tests := []struct {
		name string
		html string
		want PageMeta
	}{
		{
			name: "full head",
			html: `<html><head><title> My  App </title>
<meta property="og:title" content="OG Title"><meta property="og:description" content="About it">
<meta property="og:image" content="/og.png"><meta name="twitter:image" content="/tw.png">
<meta name="theme-color" content="#112233">
<link rel="icon" href="/favicon.ico"><link rel="apple-touch-icon" href="/apple.png"></head><body></body></html>`,
			want: PageMeta{Title: "My App", OGTitle: "OG Title", Description: "About it", OGImage: "/og.png", TwitterImage: "/tw.png", ThemeColor: "#112233", Icon: "/apple.png"},
		},
		{name: "missing tags", html: `<html><head></head><body></body></html>`, want: PageMeta{}},
		{
			name: "twitter only and name description",
			html: `<head><meta name="description" content="plain"><meta name="twitter:image" content="https://cdn.example.com/t.jpg"></head>`,
			want: PageMeta{Description: "plain", TwitterImage: "https://cdn.example.com/t.jpg"},
		},
		{name: "css theme color is dropped", html: `<head><meta name="theme-color" content="url(javascript:x)"></head>`, want: PageMeta{}},
		{name: "tags in body are ignored", html: `<body><meta property="og:image" content="/late.png"></body>`, want: PageMeta{}},
		{name: "rel token list", html: `<head><link rel="shortcut icon" href="/s.ico"></head>`, want: PageMeta{Icon: "/s.ico"}},
		{name: "first og:image wins", html: `<head><meta property="og:image" content="/a.png"><meta property="og:image" content="/b.png"></head>`, want: PageMeta{OGImage: "/a.png"}},
		{name: "truncated html", html: `<head><title>Half`, want: PageMeta{Title: "Half"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ParseMeta(strings.NewReader(tc.html)); got != tc.want {
				t.Errorf("ParseMeta = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestParseMeta_LongTitleTruncated(t *testing.T) {
	got := ParseMeta(strings.NewReader("<head><title>" + strings.Repeat("x", 500) + "</title></head>"))
	if len([]rune(got.Title)) != maxTitleLen {
		t.Errorf("title length = %d, want %d", len([]rune(got.Title)), maxTitleLen)
	}
}

func TestPageMeta_ImageRefAndTitle(t *testing.T) {
	if (PageMeta{OGImage: "a", TwitterImage: "b"}).ImageRef() != "a" || (PageMeta{TwitterImage: "b"}).ImageRef() != "b" {
		t.Error("og:image must win over twitter:image")
	}
	if (PageMeta{Title: "t", OGTitle: "o"}).DisplayTitle() != "o" || (PageMeta{Title: "t"}).DisplayTitle() != "t" {
		t.Error("og:title must win over title")
	}
}

func TestResolveRef(t *testing.T) {
	base, _ := url.Parse("http://web:3000/blog/post")
	tests := []struct {
		ref     string
		want    string
		wantErr bool
	}{
		{ref: "/og.png", want: "http://web:3000/og.png"},
		{ref: "og.png", want: "http://web:3000/blog/og.png"},
		{ref: "//cdn.example.com/x.png", want: "http://cdn.example.com/x.png"},
		{ref: "https://cdn.example.com/x.png?v=1", want: "https://cdn.example.com/x.png?v=1"},
		{ref: "file:///etc/passwd", wantErr: true},
		{ref: "javascript:alert(1)", wantErr: true},
		{ref: "data:image/png;base64,AAAA", wantErr: true},
		{ref: "ftp://example.com/x.png", wantErr: true},
		{ref: "http://user:pw@example.com/x.png", wantErr: true}, //nolint:gosec // fake credentials in a URL the parser must reject
		{ref: "http://169.254.169.254/latest/meta-data", wantErr: true},
		{ref: "http://10.0.0.5/x.png", wantErr: true},
		{ref: "http://[::1]/x.png", wantErr: true},
		{ref: "", wantErr: true},
		{ref: "http://example.com/" + strings.Repeat("a", 3000), wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.ref[:min(len(tc.ref), 40)], func(t *testing.T) {
			u, err := ResolveRef(base, tc.ref)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ResolveRef(%q) = %v, want error", tc.ref, u)
				}
				return
			}
			if err != nil || u.String() != tc.want {
				t.Errorf("ResolveRef(%q) = %v, %v; want %s", tc.ref, u, err, tc.want)
			}
		})
	}
}
