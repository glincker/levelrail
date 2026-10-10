package api

import "strings"

// cleanReleaseNotes strips HTML comments (such as the release-notes markers
// the release workflow wraps notes in) and surrounding blank space. Markdown
// and GitHub alert syntax are left for the client to render safely.
func cleanReleaseNotes(body string) string {
	var b strings.Builder
	rest := body
	for {
		start := strings.Index(rest, "<!--")
		if start < 0 {
			b.WriteString(rest)
			break
		}
		b.WriteString(rest[:start])
		end := strings.Index(rest[start:], "-->")
		if end < 0 {
			break
		}
		rest = rest[start+end+len("-->"):]
	}
	return strings.TrimSpace(b.String())
}
