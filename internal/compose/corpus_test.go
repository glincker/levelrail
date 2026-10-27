package compose_test

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/catalog"
	"github.com/GLINCKER/levelrail/internal/compose"
	"gopkg.in/yaml.v3"
)

// wantUnsupported records, per fixture, the keys the parser must report.
var wantUnsupported = map[string][]string{
	"nginx_php.yaml":            {"php:expose", "php:user", "php:working_dir", "web:container_name"},
	"wordpress_mysql.yaml":      {"db:env_file", "wordpress:links"},
	"profiles_extends.yaml":     {"debug:extends", "debug:stdin_open", "debug:tty", "migrate:depends_on.restart"},
	"secrets_configs.yaml":      {},
	"deploy_resources.yaml":     {"gpu:ulimits", "worker:deploy.resources.reservations.cpus", "worker:deploy.resources.reservations.memory"},
	"healthcheck_variants.yaml": {"http:healthcheck.start_interval", "off:healthcheck.disable"},
	"anchors_extensions.yaml":   {":volumes.data", "a:logging", "b:hostname", "b:logging", "b:ports.host_ip", "b:volumes.volume"},
	"env_file_stack.yaml":       {":name", ":volumes.redis", "api:container_name", "api:env_file", "api:stop_grace_period", "api:user", "redis:container_name"},
}

func flatten(keys []compose.UnsupportedKey) []string {
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k.Service+":"+k.Key)
	}
	sort.Strings(out)
	return out
}

func TestCorpusFixtures_ReportEveryUnsupportedKey(t *testing.T) {
	files, err := filepath.Glob("testdata/corpus/*.yaml")
	if err != nil || len(files) == 0 {
		t.Fatalf("no corpus fixtures: %v", err)
	}
	for _, path := range files {
		name := filepath.Base(path)
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(path) //nolint:gosec // globbed fixture path
			if err != nil {
				t.Fatal(err)
			}
			f, err := compose.Parse(data)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			want, ok := wantUnsupported[name]
			if !ok {
				t.Fatalf("fixture %s has no entry in wantUnsupported", name)
			}
			got := flatten(f.Unsupported)
			if strings.Join(got, ",") != strings.Join(want, ",") {
				t.Errorf("unsupported = %v, want %v", got, want)
			}
			assertNothingSilent(t, data, f)
			for _, n := range f.Notices() {
				if n.Key != "" && (n.Alternative == "" || !strings.Contains(n.Message, n.Key)) {
					t.Errorf("notice for %q lacks its key or alternative: %q", n.Key, n.Message)
				}
			}
		})
	}
}

// assertNothingSilent decodes the file independently of the parser and fails
// for any service key that is neither read, an x- extension, nor reported.
func assertNothingSilent(t *testing.T, data []byte, f *compose.File) {
	t.Helper()
	var doc struct {
		Services map[string]map[string]any `yaml:"services"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	read := map[string]bool{}
	for _, k := range compose.SupportedServiceKeys() {
		read[k] = true
	}
	reported := map[string]bool{}
	for _, u := range f.Unsupported {
		reported[u.Service+":"+u.Key] = true
	}
	for svc, body := range doc.Services {
		for key := range body {
			if !read[key] && !strings.HasPrefix(key, "x-") && !reported[svc+":"+key] {
				t.Errorf("service %q key %q is ignored without being reported", svc, key)
			}
		}
	}
}

func TestCorpusCatalog_ReportEveryUnsupportedKey(t *testing.T) {
	freq := map[string]int{}
	parsed := 0
	for _, tpl := range catalog.Templates {
		f, err := compose.Parse([]byte(tpl.Compose))
		if err != nil {
			continue
		}
		parsed++
		assertNothingSilent(t, []byte(tpl.Compose), f)
		seen := map[string]bool{}
		for _, u := range f.Unsupported {
			t.Errorf("catalog template %s has an unsupported key %q on service %q", tpl.ID, u.Key, u.Service)
			if !seen[u.Key] {
				seen[u.Key] = true
				freq[u.Key]++
			}
		}
	}
	if parsed == 0 {
		t.Fatal("no catalog template parsed")
	}
	type kv struct {
		key string
		n   int
	}
	ranked := make([]kv, 0, len(freq))
	for k, n := range freq {
		ranked = append(ranked, kv{k, n})
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].n != ranked[j].n {
			return ranked[i].n > ranked[j].n
		}
		return ranked[i].key < ranked[j].key
	})
	for _, r := range ranked {
		t.Logf("unsupported key %-40s in %d of %d catalog templates", r.key, r.n, parsed)
	}
}
