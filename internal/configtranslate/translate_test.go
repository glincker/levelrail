package configtranslate

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/spec"
)

var update = os.Getenv("CONFIGTRANSLATE_UPDATE") != ""

func loadCase(t *testing.T, name string) map[string][]byte {
	t.Helper()
	dir := filepath.Join("testdata", name)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "expected.") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name())) //nolint:gosec // fixture path from testdata
		if err != nil {
			t.Fatal(err)
		}
		files[e.Name()] = b
	}
	return files
}

func render(t *testing.T, r Result) (string, string) {
	t.Helper()
	y, err := r.Fragment.YAML()
	if err != nil {
		t.Fatal(err)
	}
	var fs []string
	for _, f := range r.Findings {
		fs = append(fs, string(f.Severity)+" "+f.Code+" "+f.Subject)
	}
	return string(y), strings.Join(fs, "\n") + "\n"
}

func TestTranslateGolden(t *testing.T) {
	cases := []struct {
		name        string
		wantFormats []Format
		validSpec   bool
	}{
		{"heroku-full", []Format{FormatHeroku}, true},
		{"heroku-minimal", []Format{FormatHeroku}, true},
		{"vercel-next", []Format{FormatVercel}, true},
		{"vercel-static", []Format{FormatVercel}, true},
		{"bad-json", []Format{FormatVercel}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := Translate(loadCase(t, tc.name))
			if !reflect.DeepEqual(res.Detected, tc.wantFormats) {
				t.Fatalf("detected = %v, want %v", res.Detected, tc.wantFormats)
			}
			gotYAML, gotFindings := render(t, res)
			for name, got := range map[string]string{"expected.yaml": gotYAML, "expected.findings": gotFindings} {
				p := filepath.Join("testdata", tc.name, name)
				if update {
					if err := os.WriteFile(p, []byte(got), 0o600); err != nil {
						t.Fatal(err)
					}
					continue
				}
				want, err := os.ReadFile(p) //nolint:gosec // fixture path from testdata
				if err != nil {
					t.Fatal(err)
				}
				if string(want) != got {
					t.Errorf("%s mismatch\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
				}
			}
			if tc.validSpec {
				if _, err := spec.Parse([]byte(gotYAML)); err != nil {
					t.Errorf("proposed fragment is not a valid app spec: %v\n%s", err, gotYAML)
				}
			}
			for _, f := range res.Findings {
				if f.Code == "" || f.Message == "" || f.Fix == "" || f.Source == "" {
					t.Errorf("incomplete finding: %+v", f)
				}
				if strings.ContainsAny(f.Message+f.Fix, string(rune(0x2013))+string(rune(0x2014))) {
					t.Errorf("dash in finding text: %+v", f)
				}
			}
		})
	}
}

func TestTranslateDeterministic(t *testing.T) {
	files := loadCase(t, "heroku-full")
	for k, v := range loadCase(t, "vercel-next") {
		files[k] = v
	}
	first := Translate(files)
	for i := 0; i < 20; i++ {
		if got := Translate(files); !reflect.DeepEqual(first, got) {
			t.Fatalf("run %d differs", i)
		}
	}
	a, _ := first.Fragment.YAML()
	b, _ := Translate(files).Fragment.YAML()
	if string(a) != string(b) {
		t.Fatal("YAML differs between runs")
	}
}

func TestDetect(t *testing.T) {
	tests := []struct {
		name  string
		files map[string][]byte
		want  []Format
	}{
		{"none", map[string][]byte{"README.md": nil}, nil},
		{"procfile", map[string][]byte{"Procfile": nil}, []Format{FormatHeroku}},
		{"appjson only", map[string][]byte{"./app.json": nil}, []Format{FormatHeroku}},
		{"both", map[string][]byte{"vercel.json": nil, "Procfile": nil}, []Format{FormatHeroku, FormatVercel}},
		{"nested ignored", map[string][]byte{"sub/vercel.json": nil}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Detect(tc.files); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Detect = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestFindingCodes(t *testing.T) {
	tests := []struct {
		name  string
		files map[string][]byte
		want  []string
	}{
		{"edge runtime", map[string][]byte{"vercel.json": []byte(`{"functions":{"a.ts":{"runtime":"edge"}}}`)}, []string{"blocker vercel.runtime.edge"}},
		{"unknown addon", map[string][]byte{"app.json": []byte(`{"addons":["bonsai:sandbox"]}`)}, []string{"warn heroku.addon.manual"}},
		{"bad procfile line", map[string][]byte{"Procfile": []byte("web node x\nworker: run\n")}, []string{"warn heroku.procfile.invalid"}},
		{"release without web", map[string][]byte{"Procfile": []byte("worker: w\nrelease: r\n")}, []string{"warn heroku.release.no-web"}},
		{"two postgres", map[string][]byte{"app.json": []byte(`{"addons":["heroku-postgresql:a",{"plan":"heroku-postgresql:b","as":"REPLICA"}]}`)}, []string{"info heroku.addon.database"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := Translate(tc.files)
			var got []string
			for _, f := range res.Findings {
				got = append(got, string(f.Severity)+" "+f.Code)
			}
			for _, w := range tc.want {
				found := false
				for _, g := range got {
					found = found || g == w
				}
				if !found {
					t.Errorf("missing %q in %v", w, got)
				}
			}
		})
	}
}

func TestMergeClash(t *testing.T) {
	files := map[string][]byte{"Procfile": []byte("web: x\n"), "vercel.json": []byte(`{"framework":"nextjs"}`)}
	res := Translate(files)
	if res.Fragment.Services["web"].Command == nil {
		t.Error("first source (Heroku) should win the clash")
	}
	var seen bool
	for _, f := range res.Findings {
		seen = seen || f.Code == "merge.service-clash"
	}
	if !seen {
		t.Error("expected merge.service-clash finding")
	}
}

func TestPortOptionsOverrideDefaults(t *testing.T) {
	files := map[string][]byte{"Procfile": []byte("web: node server.js\n")}
	def, _ := Translate(files).Fragment.YAML()
	got, _ := Translate(files, WithWebPort(9191), WithNodePort(0)).Fragment.YAML()
	if !strings.Contains(string(def), "8080") || !strings.Contains(string(got), "9191") || strings.Contains(string(got), "8080") {
		t.Fatalf("default:\n%s\noverride:\n%s", def, got)
	}
}
