package staticroutes

import (
	"strings"
	"testing"
)

func TestParseHeaders(t *testing.T) {
	src := "# comment\n/*\n  X-Frame-Options: DENY\n  X-Content-Type-Options: nosniff\n\n" +
		"/static/*\n\tCache-Control: public, max-age=31536000, immutable\n  ! Server\n" +
		"/:locale/page\n  X-Locale: set\n  Link: </a.css>; rel=preload\n  Link: </b.css>; rel=preload\n"
	blocks, probs := ParseHeaders(src, DefaultLimits())
	if len(probs) != 0 || len(blocks) != 3 {
		t.Fatalf("blocks=%d probs=%v", len(blocks), probs)
	}
	if blocks[0].Source != "_headers:2" || len(blocks[0].Set) != 2 || blocks[0].Set[0].Name != "X-Frame-Options" {
		t.Fatalf("block 0: %+v", blocks[0])
	}
	if blocks[1].Set[0].Value != "public, max-age=31536000, immutable" || len(blocks[1].Remove) != 1 || blocks[1].Remove[0] != "Server" {
		t.Fatalf("block 1: %+v", blocks[1])
	}
}

func TestParseHeadersProblems(t *testing.T) {
	tests := []struct {
		name, src, want string
		line            int
	}{
		{"header before path", "  X-A: b\n", "no path above", 1},
		{"no value", "/a\n  X-A:\n", "must be Name: value", 2},
		{"no colon", "/a\n  X-A b\n", "must be Name: value", 2},
		{"bad name", "/a\n  X A: b\n", "valid header name", 2},
		{"hop by hop", "/a\n  Transfer-Encoding: chunked\n", "managed by the ingress", 2},
		{"host header", "/a\n  Host: evil\n", "managed by the ingress", 2},
		{"forwarded", "/a\n  X-Forwarded-For: 1.1.1.1\n", "X-Forwarded", 2},
		{"cache status", "/a\n  Cache-Status: hit\n", "reserved", 2},
		{"control char value", "/a\n  X-A: b\x01c\n", "single line", 2},
		{"bad path", "relative\n  X-A: b\n", "must start with /", 1},
		{"path with space", "/a b\n  X-A: b\n", "without spaces", 1},
		{"remove without name", "/a\n  !\n", "empty", 2},
		{"splat middle", "/a/*/b\n  X-A: b\n", "last segment", 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			blocks, probs := ParseHeaders(tc.src, DefaultLimits())
			if len(probs) != 1 || !strings.Contains(probs[0].Message, tc.want) || probs[0].Line != tc.line {
				t.Fatalf("want %q on line %d, got %+v", tc.want, tc.line, probs)
			}
			for _, b := range blocks {
				if len(b.Set) != 0 && tc.name != "remove without name" {
					t.Fatalf("an invalid line must not be applied: %+v", b)
				}
			}
		})
	}
}

func TestParseHeadersInvalidPathSkipsItsLines(t *testing.T) {
	blocks, probs := ParseHeaders("bad\n  X-A: b\n/ok\n  X-B: c\n", DefaultLimits())
	if len(probs) != 1 || len(blocks) != 1 || blocks[0].Path != "/ok" {
		t.Fatalf("blocks=%v probs=%v", blocks, probs)
	}
}

func TestValidateHeaderBlockFromSpec(t *testing.T) {
	b := HeaderBlock{Path: "/*", Set: []HeaderKV{{Name: "x-frame-options", Value: "DENY"}}, Remove: []string{"server"}}
	if err := ValidateHeaderBlock(&b, DefaultLimits()); err != nil {
		t.Fatal(err)
	}
	if b.Set[0].Name != "X-Frame-Options" || b.Remove[0] != "Server" {
		t.Fatalf("names should be canonical: %+v", b)
	}
	bad := HeaderBlock{Path: "/*", Set: []HeaderKV{{Name: "X-A", Value: "a\r\nSet-Cookie: x=1"}}}
	if err := ValidateHeaderBlock(&bad, DefaultLimits()); err == nil {
		t.Fatal("a CRLF value must be rejected")
	}
}
