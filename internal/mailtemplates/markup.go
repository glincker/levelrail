package mailtemplates

import (
	"html"
	"regexp"
	"strings"
)

// The body markup is deliberately tiny: blank-line paragraphs, "# "
// headings, "- " lists, "> " callouts, "!button label|url" and inline
// **bold** and [label](url). Everything else is escaped text: no raw HTML
// and no images.
var (
	boldRe = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	linkRe = regexp.MustCompile(`\[([^\]]+)\]\(([^)\s]+)\)`)
)

type blockKind int

const (
	blockParagraph blockKind = iota
	blockHeading
	blockList
	blockCallout
	blockButton
)

type block struct {
	kind  blockKind
	lines []string
}

func parseBlocks(body string) []block {
	body = strings.ReplaceAll(strings.ReplaceAll(body, "\r\n", "\n"), "\r", "\n")
	var out []block
	for _, chunk := range strings.Split(body, "\n\n") {
		chunk = strings.TrimSpace(chunk)
		if chunk == "" {
			continue
		}
		lines := strings.Split(chunk, "\n")
		switch {
		case strings.HasPrefix(chunk, "# "):
			out = append(out, block{blockHeading, []string{strings.TrimPrefix(chunk, "# ")}})
		case allPrefixed(lines, "- "):
			out = append(out, block{blockList, trimPrefixAll(lines, "- ")})
		case allPrefixed(lines, "> "):
			out = append(out, block{blockCallout, trimPrefixAll(lines, "> ")})
		case len(lines) == 1 && strings.HasPrefix(chunk, "!button "):
			out = append(out, block{blockButton, []string{strings.TrimPrefix(chunk, "!button ")}})
		default:
			out = append(out, block{blockParagraph, lines})
		}
	}
	return out
}

func allPrefixed(lines []string, p string) bool {
	for _, l := range lines {
		if !strings.HasPrefix(l, p) {
			return false
		}
	}
	return true
}

func trimPrefixAll(lines []string, p string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = strings.TrimPrefix(l, p)
	}
	return out
}

// safeURL allows only http, https and mailto targets, plus a root-relative
// path (an instance with no dashboard address configured emails bare paths).
func safeURL(u string) string {
	u = strings.TrimSpace(u)
	l := strings.ToLower(u)
	if strings.HasPrefix(l, "https://") || strings.HasPrefix(l, "http://") || strings.HasPrefix(l, "mailto:") {
		return u
	}
	if strings.HasPrefix(u, "/") && !strings.HasPrefix(u, "//") {
		return u
	}
	return "#"
}

// inlineHTML renders one line of inline markup with tokens substituted.
// Link targets are substituted before escaping so a value never adds markup.
func inlineHTML(line string, values map[string]string) string {
	var sb strings.Builder
	last := 0
	for _, m := range linkRe.FindAllStringSubmatchIndex(line, -1) {
		sb.WriteString(plainInline(line[last:m[0]], values))
		target := safeURL(substitute(line[m[4]:m[5]], values, false))
		sb.WriteString(`<a href="` + html.EscapeString(target) + `" class="lnk" style="color:var(--mail-accent,#2563eb);text-decoration:underline">` + plainInline(line[m[2]:m[3]], values) + `</a>`)
		last = m[1]
	}
	sb.WriteString(plainInline(line[last:], values))
	return sb.String()
}

func plainInline(s string, values map[string]string) string {
	escaped := substitute(html.EscapeString(s), values, true)
	return boldRe.ReplaceAllString(escaped, "<strong>$1</strong>")
}

// inlineText renders one line as plain text: links become "label (url)".
func inlineText(line string, values map[string]string) string {
	line = linkRe.ReplaceAllString(line, "$1 ($2)")
	line = boldRe.ReplaceAllString(line, "$1")
	return SubstituteText(line, values)
}
