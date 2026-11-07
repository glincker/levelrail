package build

import (
	"os"
	"path/filepath"
	"regexp"

	rpplan "github.com/railwayapp/railpack/core/plan"
)

var (
	nextStandaloneRe  = regexp.MustCompile(`output\s*:\s*['"]standalone['"]`)
	nextTracingRootRe = regexp.MustCompile(`outputFileTracingRoot`)
	nextDistDirRe     = regexp.MustCompile(`\bdistDir\s*:`)
	jsBlockCommentRe  = regexp.MustCompile(`(?s)/\*.*?\*/`)
	jsLineCommentRe   = regexp.MustCompile(`(?m)(^|\s)//.*$`)
)

// nextStandaloneStart is the entry Next.js writes for output: 'standalone'.
const nextStandaloneStart = "node .next/standalone/server.js"

// nextStandaloneCopy puts the assets the standalone server does not copy
// next to it; without them every /_next/static and public file is a 404.
const nextStandaloneCopy = "if [ -d .next/standalone ]; then " +
	"mkdir -p .next/standalone/.next && rm -rf .next/standalone/.next/static && cp -R .next/static .next/standalone/.next/static; " +
	"if [ -d public ]; then rm -rf .next/standalone/public && cp -R public .next/standalone/public; fi; fi"

// nextStandaloneConfig reports whether the app's next.config selects the
// standalone output. A custom tracing root moves server.js into a nested
// folder and a custom distDir moves .next, so those apps keep the provider defaults.
func nextStandaloneConfig(sourceDir string) bool {
	for _, name := range []string{"next.config.js", "next.config.mjs", "next.config.ts", "next.config.cjs"} {
		b, err := os.ReadFile(filepath.Join(sourceDir, name)) //nolint:gosec // fixed config names under the build source dir
		if err != nil {
			continue
		}
		code := jsLineCommentRe.ReplaceAll(jsBlockCommentRe.ReplaceAll(b, nil), nil)
		return nextStandaloneRe.Match(code) && !nextTracingRootRe.Match(code) && !nextDistDirRe.Match(code)
	}
	return false
}

// NextRunsStandalone reports whether the Railpack plan will start the standalone
// server for this source, so callers can place the image cache where it writes.
func NextRunsStandalone(sourceDir string) bool { return nextStandaloneConfig(sourceDir) }

// applyNextStandalone makes the Railpack plan run the standalone server
// with its static and public files in place. It returns a log line, or ""
// when the app is not a standalone Next.js app.
func applyNextStandalone(plan *rpplan.BuildPlan, sourceDir string) string {
	if !nextStandaloneConfig(sourceDir) {
		return ""
	}
	for i := range plan.Steps {
		if plan.Steps[i].Name != "build" {
			continue
		}
		plan.Steps[i].Commands = append(plan.Steps[i].Commands, rpplan.NewExecShellCommand(nextStandaloneCopy))
		plan.Deploy.StartCmd = nextStandaloneStart
		// The standalone server binds to the container hostname unless told otherwise.
		if plan.Deploy.Variables == nil {
			plan.Deploy.Variables = map[string]string{}
		}
		if _, set := plan.Deploy.Variables["HOSTNAME"]; !set {
			plan.Deploy.Variables["HOSTNAME"] = "0.0.0.0"
		}
		return "next.js: output standalone, copying .next/static and public next to the standalone server"
	}
	return ""
}
