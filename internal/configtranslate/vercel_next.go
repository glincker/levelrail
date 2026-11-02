package configtranslate

import "regexp"

var nextConfigNames = []string{"next.config.js", "next.config.mjs", "next.config.ts", "next.config.cjs"}

// nextStandaloneRe matches an output option set to standalone, so a comment or
// an unrelated word does not count.
var nextStandaloneRe = regexp.MustCompile(`output\s*[:=]\s*[^,;}\n]*['"` + "`" + `]standalone['"` + "`" + `]`)

// nextStandaloneFinding hints at output: "standalone" for a Next.js server,
// which keeps the container image small. It stays quiet when the config has it.
func nextStandaloneFinding(files map[string][]byte, fw string, isStatic bool) []Finding {
	if fw != "nextjs" || isStatic {
		return nil
	}
	for _, n := range nextConfigNames {
		if b, ok := lookup(files, n); ok && nextStandaloneRe.Match(b) {
			return nil
		}
	}
	return []Finding{{Code: "vercel.next.standalone", Severity: SeverityInfo, Source: fileVercel, Subject: "next.config",
		Message: "Next.js is not set to output: standalone, the image carries the full node_modules",
		Fix:     `Add output: "standalone" to next.config for a smaller image and faster starts.`}}
}
