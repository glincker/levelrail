package configtranslate

import (
	"fmt"
	"strings"
)

// sourceFindings flags each built service: the config files name no git repo
// or image registry, and apps create refuses a source build without one.
func sourceFindings(f Fragment) []Finding {
	var fs []Finding
	for _, k := range sortedKeys(f.Services) {
		switch f.Services[k].Build.Type {
		case "railpack", "dockerfile":
			fs = append(fs, Finding{Code: "config.source.unknown", Severity: SeverityWarn, Source: "app.yaml", Subject: k,
				Message: "the source config names no git repository or image, a source build needs both --repo and --image-repo",
				Fix:     "Pass --repo and --image-repo to apps create, or build and push an image and set build.type image with build.image."})
		}
	}
	return fs
}

// NextSteps returns the exact commands that turn the proposal into running
// apps. prog is the CLI name, specFile the path the proposal was saved to.
func NextSteps(f Fragment, prog, specFile string) []string {
	var out []string
	multi := len(f.Services) > 1
	if len(f.Databases) > 0 {
		for _, k := range sortedKeys(f.Databases) {
			out = append(out, fmt.Sprintf("%s databases create --name %s --engine %s --version VERSION", prog, k, f.Databases[k].Engine))
		}
		out = append(out, "apps create does not apply databases: or env from: references. Create the databases above, delete those sections from "+specFile+", then run databases link NAME APP for each app so its connection variable is injected.")
	}
	for _, k := range sortedKeys(f.Services) {
		svc := f.Services[k]
		var b strings.Builder
		fmt.Fprintf(&b, "%s apps create --file %s", prog, specFile)
		if multi {
			fmt.Fprintf(&b, " --service %s", k)
		}
		fmt.Fprintf(&b, " --name APP")
		switch svc.Build.Type {
		case "railpack", "dockerfile":
			b.WriteString(" --repo GIT_URL --image-repo REGISTRY/IMAGE")
		}
		for _, ek := range sortedKeys(svc.Env) {
			if svc.Env[ek].Secret {
				fmt.Fprintf(&b, " --secret %s=VALUE", ek)
			}
		}
		out = append(out, b.String())
	}
	return out
}
