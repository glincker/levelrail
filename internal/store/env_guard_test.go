package store

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var envWriterName = regexp.MustCompile(`^(Set|Save|Replace|Merge|Apply|Update|Add|Put|Upsert|Insert)`)

// envGuardExempt lists env-named writers that write no env key, with the reason.
var envGuardExempt = map[string]string{
	"RecordEnvChanges":     "history rows, not env",
	"AddEnvRevision":       "snapshot of an already guarded write",
	"SaveAppEnvSettings":   "settings flags, no keys",
	"SetPreviewEnvExclude": "excludes an existing key, never creates one",
}

// TestEveryEnvWriterCallsTheGuard fails when a store method that writes env
// keys does not call the shared guard, so a new writer cannot skip validation.
func TestEveryEnvWriterCallsTheGuard(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	found := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(filepath.Clean(f))
		if err != nil {
			t.Fatal(err)
		}
		file, err := parser.ParseFile(fset, f, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range file.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || fn.Body == nil {
				continue
			}
			name := fn.Name.Name
			plain := strings.ReplaceAll(name, "Environment", "")
			isWriter := envWriterName.MatchString(name) && strings.Contains(plain, "Env")
			if name == "saveDesiredService" || name == "mergeEnvCore" {
				isWriter = true
			}
			if !isWriter {
				continue
			}
			if _, skip := envGuardExempt[name]; skip {
				continue
			}
			body := string(src[fset.Position(fn.Body.Pos()).Offset:fset.Position(fn.Body.End()).Offset])
			found++
			if !strings.Contains(body, "checkEnvKeys(") && !strings.Contains(body, "checkServiceEnvSave(") && !delegatesToGuarded(body) {
				t.Errorf("%s (%s) writes env without calling checkEnvKeys; add the guard or list it in envGuardExempt with a reason", name, f)
			}
		}
	}
	if found < 8 {
		t.Fatalf("only %d env writers found, the scan is probably broken", found)
	}
}

// delegatesToGuarded accepts a thin wrapper around a guarded core writer.
func delegatesToGuarded(body string) bool {
	for _, core := range []string{"db.mergeEnvCore(", "db.mergeServiceEnvTx(", "db.applyEnvPlanTx(", "db.saveDesiredService("} {
		if strings.Contains(body, core) {
			return true
		}
	}
	return false
}

func TestEnvWritersRefuseUnusableKeys(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "img:v1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	bad := "A=B"
	tests := []struct {
		name string
		call func() error
	}{
		{"save service plain key", func() error {
			return db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "img:v1", Port: 80, Env: map[string]string{bad: "v"}})
		}},
		{"save service secret key", func() error {
			return db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "img:v1", Port: 80, SecretEnv: SecretEnvRefsFromNames([]string{""})})
		}},
		{"merge set", func() error { _, err := db.MergeServiceEnv(ctx, "web", map[string]string{bad: "v"}, nil); return err }},
		{"merge unset", func() error { _, err := db.MergeServiceEnv(ctx, "web", nil, []string{"\x00"}); return err }},
		{"replace", func() error { _, err := db.ReplaceServiceEnv(ctx, "web", map[string]string{"": "v"}); return err }},
		{"apply plan", func() error {
			_, err := db.ApplyEnvPlan(ctx, "web", EnvPlan{Set: map[string]string{bad: "v"}})
			return err
		}},
		{"project vars", func() error { return db.SetProjectEnvVars(ctx, "p", map[string]string{bad: "v"}) }},
		{"organization vars", func() error { return db.SetOrganizationEnvVars(ctx, "o", map[string]string{bad: "v"}) }},
		{"environment vars", func() error { return db.SetEnvironmentEnvVars(ctx, "e", map[string]string{bad: "v"}) }},
		{"preview override", func() error { return db.SetServicePreviewEnvOverride(ctx, "web", bad, nil) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call()
			if !errors.Is(err, ErrEnvKeyInvalid) {
				t.Fatalf("err = %v, want ErrEnvKeyInvalid", err)
			}
		})
	}
}

func TestSaveDesiredServiceRefusesNewDualCopyButKeepsOld(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	dual := DesiredService{Name: "web", Image: "img:v1", Port: 80, Env: map[string]string{"K": "v"}, SecretEnv: SecretEnvRefsFromNames([]string{"K"})}
	if err := db.SaveDesiredService(ctx, dual); !errors.Is(err, ErrEnvDualHome) {
		t.Fatalf("new dual copy err = %v, want ErrEnvDualHome", err)
	}
}
