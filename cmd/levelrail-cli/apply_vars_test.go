package main

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/iac"
)

func placeholderFile(names ...string) []apiclient.IaCFile {
	var b strings.Builder
	b.WriteString("version: 1\nkind: Project\nmetadata: {name: p}\nspec:\n  env:\n")
	for _, n := range names {
		b.WriteString("    " + n + ": \"${{ env." + n + " }}\"\n")
	}
	return []apiclient.IaCFile{{Name: "p.yaml", Content: b.String()}}
}

func TestResolveVars(t *testing.T) {
	varFile := filepath.Join(t.TempDir(), "vars.env")
	if err := os.WriteFile(varFile, []byte("# comment\nexport REGION=from-file\nTIER='gold'\n\nAWS_SECRET_ACCESS_KEY=file-ok\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{
		"REGION": "from-env", "TIER": "env-tier", "APP_COLOR": "blue", "APP_SIZE": "l",
		"S3_ACCESS_KEY_ID": "s3leak", "APP_DB_PASSWORD": "pwleak",
		"AWS_SECRET_ACCESS_KEY": "leak", "AWS_REGION": "us-east-1", "GITHUB_TOKEN": "ghs_leak", "HOME": "/root",
	}
	lookup := func(k string) (string, bool) { v, ok := env[k]; return v, ok }

	cases := []struct {
		name     string
		names    []string
		src      varSources
		want     map[string]string
		errHas   []string
		errLacks []string
	}{
		{name: "no placeholders", want: map[string]string{}},
		{name: "unresolved without any source fails", names: []string{"REGION"},
			errHas: []string{"REGION: pass --var REGION=VALUE", "--allow-env REGION"}},
		{name: "environment not read without allowlist", names: []string{"HOME"}, errHas: []string{"HOME"}, errLacks: []string{"/root"}},
		{name: "--var wins over file and env", names: []string{"REGION"},
			src:  varSources{vars: map[string]string{"REGION": "from-flag"}, varFiles: []string{varFile}, allowEnv: []string{"REGION"}},
			want: map[string]string{"REGION": "from-flag"}},
		{name: "--var-file wins over env and parses dotenv", names: []string{"REGION", "TIER"},
			src:  varSources{varFiles: []string{varFile}, allowEnv: []string{"REGION,TIER"}},
			want: map[string]string{"REGION": "from-file", "TIER": "gold"}},
		{name: "allowlist reads only listed names", names: []string{"REGION", "TIER"},
			src: varSources{allowEnv: []string{"REGION"}}, errHas: []string{"TIER"}, errLacks: []string{"env-tier"}},
		{name: "prefix wildcard", names: []string{"APP_COLOR", "APP_SIZE"},
			src: varSources{allowEnv: []string{"APP_*"}}, want: map[string]string{"APP_COLOR": "blue", "APP_SIZE": "l"}},
		{name: "credential blocked by wildcard", names: []string{"AWS_SECRET_ACCESS_KEY"},
			src: varSources{allowEnv: []string{"AWS_*"}}, errHas: []string{"credential"}, errLacks: []string{"leak"}},
		{name: "credential prefix blocked by wildcard", names: []string{"AWS_REGION"},
			src: varSources{allowEnv: []string{"A*"}}, errHas: []string{"AWS_REGION", "credential"}},
		{name: "GITHUB_TOKEN blocked by wildcard", names: []string{"GITHUB_TOKEN"},
			src: varSources{allowEnv: []string{"GITHUB_*"}}, errHas: []string{"GITHUB_TOKEN"}, errLacks: []string{"ghs_leak"}},
		{name: "S3 key blocked by wildcard", names: []string{"S3_ACCESS_KEY_ID"},
			src: varSources{allowEnv: []string{"S3_*"}}, errHas: []string{"credential"}, errLacks: []string{"s3leak"}},
		{name: "secret looking name blocked by app wildcard", names: []string{"APP_DB_PASSWORD", "APP_COLOR"},
			src: varSources{allowEnv: []string{"APP_*"}}, errHas: []string{"APP_DB_PASSWORD"}, errLacks: []string{"pwleak", "APP_COLOR:"}},
		{name: "explicit credential allow works", names: []string{"AWS_SECRET_ACCESS_KEY"},
			src: varSources{allowEnv: []string{"AWS_SECRET_ACCESS_KEY"}}, want: map[string]string{"AWS_SECRET_ACCESS_KEY": "leak"}},
		{name: "credential via var file is explicit", names: []string{"AWS_SECRET_ACCESS_KEY"},
			src: varSources{varFiles: []string{varFile}}, want: map[string]string{"AWS_SECRET_ACCESS_KEY": "file-ok"}},
		{name: "allowed but unset", names: []string{"NOT_SET"},
			src: varSources{allowEnv: []string{"NOT_SET"}}, errHas: []string{"not set in the environment"}},
		{name: "bad allow-env name", names: []string{"REGION"},
			src: varSources{allowEnv: []string{"bad name"}}, errHas: []string{"not a variable name"}},
		{name: "missing var file", names: []string{"REGION"},
			src: varSources{varFiles: []string{filepath.Join(t.TempDir(), "nope")}}, errHas: []string{"--var-file"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveVars(placeholderFile(tc.names...), tc.src, lookup)
			if len(tc.errHas) > 0 {
				if err == nil {
					t.Fatalf("want error, got %v", got)
				}
				for _, s := range tc.errHas {
					if !strings.Contains(err.Error(), s) {
						t.Errorf("error lacks %q: %v", s, err)
					}
				}
				for _, s := range tc.errLacks {
					if strings.Contains(err.Error(), s) {
						t.Errorf("error leaks %q: %v", s, err)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("resolve: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("vars = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestApply_HostilePlaceholderNeverReachesServer(t *testing.T) {
	f := &iacFake{plan: pendingPlan()}
	srv := httptest.NewServer(f.handler(t))
	defer srv.Close()
	path := filepath.Join(t.TempDir(), "shared.yaml")
	hostile := "version: 1\nkind: Project\nmetadata: {name: p}\nspec:\n  env: {X: \"${{ env.AWS_SECRET_ACCESS_KEY }}\", R: \"${{ env.REGION }}\"}\n"
	if err := os.WriteFile(path, []byte(hostile), 0o600); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"AWS_SECRET_ACCESS_KEY": "wJalrXUtnFEMI", "REGION": "eu"} //nolint:gosec // a fake value the test checks is never sent

	for _, args := range [][]string{
		{"apply", "-f", path, "--yes", "--api-url", srv.URL},
		{"diff", "-f", path, "--allow-env", "REGION", "--api-url", srv.URL},
		{"apply", "-f", path, "--dry-run", "--allow-env", "*", "--api-url", srv.URL},
		{"apply", "-f", path, "--dry-run", "--allow-env", "AWS_*,REGION", "--api-url", srv.URL},
	} {
		code, out, stderr := runIaC(args, env)
		named := strings.Contains(stderr, "AWS_SECRET_ACCESS_KEY") || strings.Contains(stderr, "not a variable name")
		if code != iacExitError || !named {
			t.Fatalf("%v: code = %d stderr = %s", args, code, stderr)
		}
		if strings.Contains(out+stderr, "wJalrXUtnFEMI") {
			t.Fatalf("%v: value printed", args)
		}
	}
	if len(f.requests) != 0 {
		t.Fatalf("server received %d request(s) with unresolved placeholders", len(f.requests))
	}

	code, _, stderr := runIaC([]string{"apply", "-f", path, "--dry-run", "--allow-env", "AWS_SECRET_ACCESS_KEY,REGION", "--api-url", srv.URL}, env)
	if code != exitOK || len(f.requests) != 1 || f.requests[0].Vars["AWS_SECRET_ACCESS_KEY"] != "wJalrXUtnFEMI" {
		t.Fatalf("explicit allow: code = %d stderr = %s requests = %d", code, stderr, len(f.requests))
	}
}

func TestExport_PlaceholderWarningExplainsHowToSupplyValues(t *testing.T) {
	f := &iacFake{export: iac.ExportResult{
		Files:    []iac.ExportFile{{Name: "app-web.yaml", Kind: iac.KindApp, Content: "version: 1\nspec: {env: {API_KEY: ${{ env.API_KEY }}}}\n"}},
		Warnings: []string{"env API_KEY looks like a secret and was written as a placeholder"},
	}}
	srv := httptest.NewServer(f.handler(t))
	defer srv.Close()
	code, _, stderr := runIaC([]string{"export", "--api-url", srv.URL}, nil)
	if code != exitOK || !strings.Contains(stderr, "--var NAME=VALUE") || !strings.Contains(stderr, "--allow-env") {
		t.Fatalf("code = %d stderr = %s", code, stderr)
	}
}

func TestResolveVars_IgnoresCommentedPlaceholders(t *testing.T) {
	files := []apiclient.IaCFile{{Name: "a.yaml", Content: "# set ${{ env.OLD_THING }} before applying\nversion: 1\n  # ${{ env.ALSO_OLD }}\nx: \"${{ env.REGION }}\"\n"}}
	got, err := resolveVars(files, varSources{vars: map[string]string{"REGION": "eu"}}, func(string) (string, bool) { return "", false })
	if err != nil || !reflect.DeepEqual(got, map[string]string{"REGION": "eu"}) {
		t.Fatalf("vars = %v err = %v", got, err)
	}
}

func TestReadVarFile_RejectsDroppedLines(t *testing.T) {
	for name, tc := range map[string]struct {
		content string
		ok      bool
	}{
		"valid":              {content: "# c\nexport A=1\nB=\"two\nlines\"\nC='x'\n", ok: true},
		"missing separator":  {content: "A=1\nexportREGION eu\n"},
		"invalid name":       {content: "A=1\n9BAD=x\n"},
		"export without key": {content: "export =1\n"},
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "v.env")
			if err := os.WriteFile(path, []byte(tc.content), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := readVarFile(path)
			if tc.ok != (err == nil) {
				t.Fatalf("err = %v, want ok %v", err, tc.ok)
			}
			if err != nil && !strings.Contains(err.Error(), "line") {
				t.Fatalf("error does not name the line: %v", err)
			}
		})
	}
}

func TestExport_NoNoteWithoutPlaceholders(t *testing.T) {
	f := &iacFake{export: iac.ExportResult{Files: []iac.ExportFile{{Name: "tag-x.yaml", Kind: iac.KindTag, Content: "version: 1\n"}}}}
	srv := httptest.NewServer(f.handler(t))
	defer srv.Close()
	code, _, stderr := runIaC([]string{"export", "--include-env-values=false", "--api-url", srv.URL}, nil)
	if code != exitOK || strings.Contains(stderr, "--allow-env") {
		t.Fatalf("code = %d stderr = %s", code, stderr)
	}
}
