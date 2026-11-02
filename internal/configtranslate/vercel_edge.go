package configtranslate

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
)

var edgeRuntimeDecl = regexp.MustCompile(`export\s+const\s+runtime\s*=\s*['"]edge['"]`)

var edgeSourceExt = map[string]bool{".ts": true, ".tsx": true, ".js": true, ".jsx": true, ".mjs": true}

// edgeSourceFiles lists source files in files that declare the edge runtime.
func edgeSourceFiles(files map[string][]byte) []string {
	var out []string
	for name, b := range files {
		if !edgeSourceExt[path.Ext(name)] || !edgeRuntimeDecl.Match(b) {
			continue
		}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// edgeSourceFinding warns, never blocks: the app still runs, as Node.
func edgeSourceFinding(names []string) Finding {
	shown := names
	if len(shown) > 5 {
		shown = shown[:5]
	}
	subject := strings.Join(shown, ", ")
	if len(names) > len(shown) {
		subject += fmt.Sprintf(" and %d more", len(names)-len(shown))
	}
	return Finding{Code: "vercel.runtime.edge.source", Severity: SeverityWarn, Source: fileVercel, Subject: subject,
		Message: "export const runtime = 'edge' found: Levelrail runs Node, so these routes run as Node. Most middleware and handlers work unchanged; edge-only APIs do not (behavior unverified here)",
		Fix:     "Remove the runtime export, then test routes that use edge-only APIs such as @vercel/edge or @vercel/og."}
}
