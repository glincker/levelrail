package ghactions

import (
	"fmt"
	"strings"
)

// SkipMarkers are the commit message markers that stop a push from deploying.
var SkipMarkers = []string{"[skip ci]", "[ci skip]", "[skip deploy]", "[no deploy]"}

// WorkflowOptions describes the workflow to generate.
type WorkflowOptions struct {
	App         string
	APIURL      string
	Branch      string
	Environment string
	Paths       []string
	IgnorePaths []string
	ActionRef   string
}

func yamlList(items []string, indent string) string {
	var b strings.Builder
	for _, it := range items {
		fmt.Fprintf(&b, "%s- %q\n", indent, it)
	}
	return b.String()
}

// Workflow renders a deploy-on-push workflow that authenticates with the job OIDC identity,
// waits for the rollout and rolls back to the previous image on failure.
func Workflow(o WorkflowOptions) string {
	if o.Branch == "" {
		o.Branch = "main"
	}
	if o.Environment == "" {
		o.Environment = "production"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "name: Deploy %s\n", o.App)
	b.WriteString("on:\n  push:\n    branches: [" + o.Branch + "]\n")
	if len(o.Paths) > 0 {
		b.WriteString("    paths:\n" + yamlList(o.Paths, "      "))
	}
	if len(o.IgnorePaths) > 0 {
		b.WriteString("    paths-ignore:\n" + yamlList(o.IgnorePaths, "      "))
	}
	b.WriteString("  workflow_dispatch:\n\n")
	b.WriteString("permissions:\n  contents: read\n  packages: write\n  id-token: write\n  deployments: write\n  checks: write\n\n")
	fmt.Fprintf(&b, "concurrency:\n  group: deploy-%s\n  cancel-in-progress: false\n\n", o.App)
	b.WriteString("jobs:\n  deploy:\n")
	var skips []string
	for _, m := range SkipMarkers {
		skips = append(skips, fmt.Sprintf("!contains(github.event.head_commit.message, '%s')", m))
	}
	fmt.Fprintf(&b, "    if: ${{ %s }}\n", strings.Join(skips, " && "))
	fmt.Fprintf(&b, "    runs-on: ubuntu-latest\n    environment: %s\n    steps:\n", o.Environment)
	b.WriteString("      - uses: actions/checkout@v4\n")
	b.WriteString("      - uses: docker/login-action@v3\n        with:\n          registry: ghcr.io\n          username: ${{ github.actor }}\n          password: ${{ secrets.GITHUB_TOKEN }}\n")
	b.WriteString("      - uses: docker/build-push-action@v6\n        with:\n          context: .\n          push: true\n          tags: ghcr.io/${{ github.repository }}:${{ github.sha }}\n")
	fmt.Fprintf(&b, "      - uses: %s\n        with:\n", o.ActionRef)
	fmt.Fprintf(&b, "          api-url: %s\n          oidc: \"true\"\n          rollback-on-failure: \"true\"\n          app: %s\n", o.APIURL, o.App)
	b.WriteString("          image: ghcr.io/${{ github.repository }}:${{ github.sha }}\n")
	return b.String()
}
