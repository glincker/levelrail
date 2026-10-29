package k8simport

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/iac"
)

var update = flag.Bool("update", false, "rewrite golden files")

// Secret documents live here, not in testdata, so fixture files hold no key material.
var testSecrets = map[string]string{
	"nginx.yaml": `---
apiVersion: v1
kind: Secret
metadata:
  name: web-secret
  namespace: shop
type: Opaque
stringData:
  API_TOKEN: fixture-token-aaa
data:
  DB_PASSWORD: Zml4dHVyZS1kYi1iYmI=
`,
	"postgres.yaml": `---
apiVersion: v1
kind: Secret
metadata:
  name: pg
data:
  POSTGRES_PASSWORD: Zml4dHVyZS1wZy1jY2M=
`,
}

func load(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name)) //nolint:gosec // fixed fixture dir
	if err != nil {
		t.Fatal(err)
	}
	return append(b, []byte("\n"+testSecrets[name])...)
}

func golden(t *testing.T, name, got string) {
	t.Helper()
	p := filepath.Join("testdata", name+".golden")
	if *update {
		if err := os.WriteFile(p, []byte(got), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(p) //nolint:gosec // fixed fixture dir
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != got {
		t.Errorf("%s differs from golden:\n%s", name, got)
	}
}

func convert(t *testing.T, name string, o Options) *Result {
	t.Helper()
	res, err := Convert(load(t, name), o)
	if err != nil {
		t.Fatalf("convert %s: %v", name, err)
	}
	return res
}

func reportText(r *Result) string {
	var b strings.Builder
	for _, f := range r.Findings {
		b.WriteString(string(f.Severity) + " " + f.Kind + "/" + f.Name + " " + f.Field + ": " + f.Reason + "\n")
	}
	return b.String()
}

// buildsAsStack proves the documents pass the same parser apply uses.
func buildsAsStack(t *testing.T, docs string) {
	t.Helper()
	parsed, issues := iac.ParseDocuments([]iac.Source{{Name: "k8s.yaml", Data: []byte(docs)}})
	if len(issues) > 0 {
		t.Fatalf("parse issues: %+v", issues)
	}
	if _, issues := iac.Build(parsed, iac.Options{}); len(issues) > 0 {
		t.Fatalf("build issues: %+v\n%s", issues, docs)
	}
}

func TestGoldenFixtures(t *testing.T) {
	for _, name := range []string{"nginx", "postgres", "cronjob", "sidecar"} {
		t.Run(name, func(t *testing.T) {
			res := convert(t, name+".yaml", Options{})
			golden(t, name+".docs", res.Documents)
			golden(t, name+".report", reportText(res))
			buildsAsStack(t, res.Documents)
		})
	}
}

func TestHostPathIsBlocker(t *testing.T) {
	res := convert(t, "hostpath.yaml", Options{})
	for _, field := range []string{"hostNetwork", "volumes[0].hostPath", "containers[0].securityContext.privileged", "containers[0].securityContext.capabilities.add"} {
		found := false
		for _, f := range res.Findings {
			if f.Severity == SeverityBlocker && strings.HasSuffix(f.Field, field) {
				found = true
			}
		}
		if !found {
			t.Errorf("no blocker for %s:\n%s", field, reportText(res))
		}
	}
	if res.Blockers() < 4 {
		t.Fatalf("blockers = %d", res.Blockers())
	}
}

func TestSecretValuesNeverInOutput(t *testing.T) {
	res := convert(t, "nginx.yaml", Options{})
	out, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	all := string(out) + res.Documents + reportText(res)
	for _, v := range []string{"fixture-token-aaa", "fixture-db-bbb", "Zml4dHVyZS1kYi1iYmI="} {
		if strings.Contains(all, v) {
			t.Errorf("secret value %q leaked", v)
		}
	}
	if got := res.Secrets()["web/API_TOKEN"]; got != "fixture-token-aaa" {
		t.Errorf("stringData not kept for apply: %q", got)
	}
	if got := res.Secrets()["web/DB_PASSWORD"]; got != "fixture-db-bbb" {
		t.Errorf("base64 data not decoded for apply: %q", got)
	}
}

func TestNamespaceModes(t *testing.T) {
	prefix := convert(t, "nginx.yaml", Options{NamespaceAs: NamespacePrefix, AppPrefix: "k-"})
	if prefix.Apps[0].Name != "k-shop-web" || strings.Contains(prefix.Documents, "kind: Environment") {
		t.Errorf("prefix mode: %s", prefix.Documents)
	}
	ignore := convert(t, "nginx.yaml", Options{NamespaceAs: NamespaceIgnore})
	if ignore.Apps[0].Name != "web" || strings.Contains(ignore.Documents, "environment:") {
		t.Errorf("ignore mode: %s", ignore.Documents)
	}
}

func TestInputHardening(t *testing.T) {
	bomb := "a: &a [x, x, x, x, x, x, x, x, x]\n"
	prev := "a"
	for _, n := range []string{"b", "c", "d", "e", "f", "g", "h", "i", "j"} {
		bomb += n + ": &" + n + " [" + strings.TrimSuffix(strings.Repeat("*"+prev+", ", 9), ", ") + "]\n"
		prev = n
	}
	cases := map[string]struct {
		in   string
		opts Options
	}{
		"yaml bomb": {"kind: ConfigMap\nmetadata: {name: x}\ndata:\n  v: x\nextra:\n" + indent(bomb), Options{}},
		"too big":   {strings.Repeat("a: b\n", 100), Options{Limits: Limits{MaxBytes: 50}}},
		"too many":  {strings.Repeat("kind: Namespace\nmetadata: {name: n}\n---\n", 5), Options{Limits: Limits{MaxDocs: 3}}},
		"too deep":  {strings.Repeat("[", 100) + strings.Repeat("]", 100), Options{}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Convert([]byte(c.in), c.opts); err == nil {
				t.Fatal("input was accepted")
			}
		})
	}
}

func indent(s string) string {
	return "  " + strings.ReplaceAll(strings.TrimSuffix(s, "\n"), "\n", "\n  ") + "\n"
}

func TestUnsupportedKindSkipped(t *testing.T) {
	res, err := Convert([]byte("apiVersion: v1\nkind: NetworkPolicy\nmetadata: {name: deny}\nspec: {}\n"), Options{})
	if err != nil || len(res.Findings) != 1 || res.Findings[0].Severity != SeverityChangesBehavior {
		t.Fatalf("%v %+v", err, res)
	}
}
