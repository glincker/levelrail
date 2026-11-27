package importplan

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/importreport"
)

var updateReport = flag.Bool("update-report", false, "rewrite report golden files")

func reportText(rep []importreport.Finding) string {
	var b strings.Builder
	for _, f := range rep {
		fmt.Fprintf(&b, "%s | %s | %s | %s | %s\n", f.Severity, f.Kind, f.Path, f.Reason, f.Fix)
	}
	return b.String()
}

func checkReportGolden(t *testing.T, name, got string) {
	t.Helper()
	p := filepath.Join("testdata", "report", name+".golden")
	if *updateReport {
		if err := os.WriteFile(p, []byte(got), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(p) //nolint:gosec // fixed testdata path
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if string(want) != got {
		t.Errorf("%s differs from golden:\n%s", name, got)
	}
}

func TestImportReportCompose(t *testing.T) {
	for _, name := range []string{"wordpress", "netdata", "placement", "gitea", "app-build"} {
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("testdata", "report", name+".yaml")) //nolint:gosec // fixed testdata path
			if err != nil {
				t.Fatal(err)
			}
			p, err := Plan(context.Background(), Input{Text: string(data), Kind: SourceCompose}, Deps{})
			if err != nil {
				t.Fatal(err)
			}
			for _, f := range p.Report {
				if f.Reason == "" || f.Path == "" || f.Severity == "" {
					t.Errorf("incomplete finding %+v", f)
				}
			}
			checkReportGolden(t, name, reportText(p.Report))
		})
	}
}

func TestImportReportDockerRun(t *testing.T) {
	cases := map[string]string{
		"run-privileged": `docker run -d --name agent --privileged --pid host --network host --cap-add NET_ADMIN -v /var/run/docker.sock:/var/run/docker.sock netdata/netdata:latest`,
		"run-web":        `docker run -d --name web -p 8080:80 -p 9090 --platform linux/arm64 --health-cmd "curl -f http://localhost" -v /srv/site:/usr/share/nginx/html nginx:latest`,
		"run-db":         `docker run -d --name pg -e POSTGRES_PASSWORD=secret -e TZ --env-file prod.env --label app=db --hostname pg -v pgdata:/var/lib/postgresql/data postgres:16`,
	}
	for name, cmd := range cases {
		t.Run(name, func(t *testing.T) {
			p, err := Plan(context.Background(), Input{Text: cmd}, Deps{})
			if err != nil {
				t.Fatal(err)
			}
			if len(p.Report) == 0 {
				t.Fatal("empty report")
			}
			checkReportGolden(t, name, reportText(p.Report))
		})
	}
}

func TestImportReportAcceptance(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "report", "placement.yaml")) //nolint:gosec // fixed testdata path
	if err != nil {
		t.Fatal(err)
	}
	p, err := Plan(context.Background(), Input{Text: string(data), Kind: SourceCompose}, Deps{})
	if err != nil {
		t.Fatal(err)
	}
	if importreport.Blockers(p.Report) < 1 || len(p.Report) < 3 {
		t.Errorf("want a blocker and at least three rows, got %s", reportText(p.Report))
	}
}
