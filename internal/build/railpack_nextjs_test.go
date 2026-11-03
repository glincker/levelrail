package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	rpplan "github.com/railwayapp/railpack/core/plan"
)

func TestApplyNextStandalone(t *testing.T) {
	cases := []struct {
		name, cfg string
		want      bool
	}{
		{"standalone", "module.exports = { output: 'standalone' }", true},
		{"mjs double quotes", `export default { output: "standalone" }`, true},
		{"default output", "module.exports = {}", false},
		{"custom tracing root", "module.exports = { output: 'standalone', outputFileTracingRoot: '..' }", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "next.config.js"), []byte(c.cfg), 0o600); err != nil {
				t.Fatal(err)
			}
			plan := rpplan.NewBuildPlan()
			plan.Steps = []rpplan.Step{{Name: "install"}, {Name: "build"}}
			plan.Deploy.StartCmd = "npm run start"
			log := applyNextStandalone(plan, dir)
			if (log != "") != c.want {
				t.Fatalf("log = %q, want applied=%v", log, c.want)
			}
			if !c.want {
				if plan.Deploy.StartCmd != "npm run start" || len(plan.Steps[1].Commands) != 0 {
					t.Fatalf("plan changed: %+v", plan)
				}
				return
			}
			if plan.Deploy.StartCmd != "node .next/standalone/server.js" || plan.Deploy.Variables["HOSTNAME"] != "0.0.0.0" {
				t.Fatalf("deploy = %+v", plan.Deploy)
			}
			if len(plan.Steps[1].Commands) != 1 {
				t.Fatalf("build commands = %d", len(plan.Steps[1].Commands))
			}
			if !strings.Contains(nextStandaloneCopy, ".next/standalone/.next/static") || !strings.Contains(nextStandaloneCopy, ".next/standalone/public") {
				t.Fatal("copy command misses static or public")
			}
		})
	}
}
