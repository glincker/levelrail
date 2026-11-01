package secretmask_test

import (
	"context"
	"encoding/base64"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/secretmask"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Canary values are obviously fake and long enough to pass MinLength.
func TestMaskAdversarialSecretShapes(t *testing.T) {
	cases := []struct {
		name   string
		secret string
		text   string
	}{
		{"regex specials", `canary.*+?^${}()|[]\pw`, `login canary.*+?^${}()|[]\pw ok`},
		{"inside a longer word", "canarysecret99", "prefix-canarysecret99-suffix and xcanarysecret99y"},
		{"multi line value in one chunk", "-----BEGIN CANARY-----\nline-one-body\n-----END CANARY-----", "key:\n-----BEGIN CANARY-----\nline-one-body\n-----END CANARY-----\ndone"},
		{"base64 form", "canary-b64-secret", "Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte("canary-b64-secret"))},
		{"raw base64 form", "canary-b64-secret!", base64.RawStdEncoding.EncodeToString([]byte("canary-b64-secret!"))},
		{"url safe base64 form", "canary??b64>>secret", base64.URLEncoding.EncodeToString([]byte("canary??b64>>secret"))},
		{"query escaped form", "canary pw&a=b/c#d", "postgres://u:" + url.QueryEscape("canary pw&a=b/c#d") + "@db/x"},
		{"path escaped form", "canary pw/seg", "GET /" + url.PathEscape("canary pw/seg")},
		{"unicode", "canary-pässwörd-✓", "got canary-pässwörd-✓ here"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := secretmask.New()
			r.Add(tc.secret)
			got := r.Mask(tc.text)
			if strings.Contains(got, tc.secret) {
				t.Fatalf("secret survived: %q", got)
			}
			for _, form := range []string{
				base64.StdEncoding.EncodeToString([]byte(tc.secret)), base64.RawStdEncoding.EncodeToString([]byte(tc.secret)),
				base64.URLEncoding.EncodeToString([]byte(tc.secret)), url.QueryEscape(tc.secret), url.PathEscape(tc.secret),
			} {
				if form != tc.secret && strings.Contains(got, form) {
					t.Fatalf("encoded form %q survived: %q", form, got)
				}
			}
			if !strings.Contains(got, secretmask.Marker) {
				t.Fatalf("no marker in %q", got)
			}
		})
	}
}

// Policy: values shorter than MinLength are never masked, because masking "true" or "1234" would shred
// ordinary text. Operators relying on short secrets get no log protection, which the docs must say.
func TestMaskShortSecretPolicy(t *testing.T) {
	r := secretmask.New()
	short := strings.Repeat("x", secretmask.MinLength-1)
	r.Add(short)
	if got := r.Mask("pw=" + short); got != "pw="+short {
		t.Fatalf("short value was masked: %q (policy is MinLength=%d)", got, secretmask.MinLength)
	}
	boundary := strings.Repeat("y", secretmask.MinLength)
	r.Add(boundary)
	if strings.Contains(r.Mask(boundary), boundary) {
		t.Fatal("a value of exactly MinLength must be masked")
	}
}

// A longer secret that contains a shorter one must not leave the tail behind.
func TestMaskLongestFirst(t *testing.T) {
	r := secretmask.New()
	r.Add("canary-prefix")
	r.Add("canary-prefix-and-more-tail")
	if got := r.Mask("v=canary-prefix-and-more-tail"); strings.Contains(got, "more-tail") {
		t.Fatalf("tail leaked: %q", got)
	}
}

// A store write path that stores free text must mask it. Every file that inserts into one of these
// tables either calls secretmask or is listed in unreviewedSinks with the reason it has not been proven.
var freeTextTables = regexp.MustCompile(`INSERT (?:OR [A-Z]+ )?INTO (log_entries|deploy_logs|audit_log|pipeline_logs|app_events|deploy_trace_steps|error_events|notification_deliveries|webhook_deliveries|event_webhook_deliveries|status_webhook_deliveries|pipeline_webhook_deliveries|ai_chat_messages|email_deliveries|alert_history|service_hook_runs|scheduled_task_runs|oneshot_jobs|build_runs|pipeline_jobs|gate_events|deploy_gate_events|team_events|node_security_events|database_query_history)\b`)

// unreviewedSinks is debt, not an approval: each path stores text that could carry a value, and no
// proof test drives it yet. Remove an entry when the file masks or a test proves the field is structured.
var unreviewedSinks = map[string]string{
	"internal/alerting/notification_delivery.go": "delivery response bodies",
	"internal/alerting/history.go":               "alert message text",
	"internal/telemetry/errors.go":               "raw error event payload from the app",
	"internal/rollback/host.go":                  "rollback event detail",
	"internal/store/ai_chat.go":                  "chat messages",
	"internal/store/deploy_gate.go":              "gate decision detail",
	"internal/store/gate.go":                     "gate decision detail",
	"internal/store/event_webhook.go":            "delivery response excerpt",
	"internal/store/webhook_delivery.go":         "inbound webhook delivery excerpt",
	"internal/store/status_webhook.go":           "delivery response excerpt",
	"internal/store/pipeline_webhooks.go":        "inbound webhook delivery excerpt",
	"internal/store/database_queries.go":         "user SQL text",
	"internal/store/email_deliveries.go":         "provider error text",
	"internal/store/team_invites.go":             "team event detail",
	"internal/store/oneshot_jobs.go":             "job message",
	"internal/store/node_security.go":            "node security event detail",
}

// reviewedWriters are functions that write to a free-text table without a Mask call in their own body,
// each with the reason it is safe. Keyed "file:Func".
var reviewedWriters = map[string]string{
	"internal/store/scheduled_task_run.go:ClaimScheduledTaskRun":             "masked in runArgs",
	"internal/store/scheduled_task_run.go:CreateScheduledTaskRun":            "masked in runArgs",
	"internal/store/scheduled_task_run.go:InterruptRunningScheduledTaskRuns": "fixed message text",
	"internal/store/scheduled_task_run.go:PruneScheduledTaskRuns":            "clears output, writes none",
	"internal/store/deploy_attempt.go:FailOrphanedDeployAttempts":            "fixed restart message",
	"internal/store/deploy_attempt_safety.go:RecoverRestartRollout":          "clears the error",
	"internal/store/pipeline_jobs.go:CreatePipelineJobs":                     "structured job fields, no output",
}

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// freeTextUpdates covers UPDATE statements that overwrite a free-text column of an existing row.
var freeTextUpdates = regexp.MustCompile(`(?s)UPDATE (?:scheduled_tasks\s+SET[^;]*?last_run_output|deploy_attempts\s+SET[^;]*?\berror\b|pipeline_jobs\s+SET[^;]*?\boutputs\b|scheduled_task_runs\s+SET[^;]*?\b(?:output|message)\b|build_runs\s+SET[^;]*?\b(?:output|error)\b|service_hook_runs\s+SET[^;]*?\b(?:output|error)\b)`)

// funcsWritingFreeText returns the functions of one file whose body holds a free-text SQL write
// and whether each also calls into secretmask inside that same body.
func funcsWritingFreeText(t *testing.T, path string) (writers map[string]bool) {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	writers = map[string]bool{}
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		writes, masks := false, false
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.BasicLit:
				if x.Kind == token.STRING && (freeTextTables.MatchString(x.Value) || freeTextUpdates.MatchString(x.Value)) {
					writes = true
				}
			case *ast.SelectorExpr:
				if id, ok := x.X.(*ast.Ident); ok && id.Name == "secretmask" {
					masks = true
				}
			}
			return true
		})
		if writes {
			writers[fn.Name.Name] = masks
		}
	}
	return writers
}

func TestEveryFreeTextStoreSinkMasksOrIsListed(t *testing.T) {
	root := repoRoot(t)
	seen := map[string]bool{}
	for _, dir := range []string{"internal", "cmd"} {
		_ = filepath.WalkDir(filepath.Join(root, dir), func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
				return nil
			}
			b, _ := os.ReadFile(p) //nolint:gosec // walks this repo's own source tree
			if !freeTextTables.Match(b) && !freeTextUpdates.Match(b) {
				return nil
			}
			rel, _ := filepath.Rel(root, p)
			rel = filepath.ToSlash(rel)
			writers := funcsWritingFreeText(t, p)
			if len(writers) == 0 {
				return nil
			}
			seen[rel] = true
			if _, listed := unreviewedSinks[rel]; listed {
				return nil
			}
			for fn, masks := range writers {
				if _, ok := reviewedWriters[rel+":"+fn]; !masks && !ok {
					t.Errorf("%s: %s writes free text but never calls secretmask in its own body; mask it or list the file in unreviewedSinks", rel, fn)
				}
			}
			return nil
		})
	}
	for key := range reviewedWriters {
		rel, fn, _ := strings.Cut(key, ":")
		if _, ok := funcsWritingFreeText(t, filepath.Join(root, rel))[fn]; !ok {
			t.Errorf("reviewedWriters lists %s which no longer writes free text; drop the entry", key)
		}
	}
	for rel := range unreviewedSinks {
		if !seen[rel] {
			t.Errorf("unreviewedSinks lists %s which no longer writes a free-text table; drop the entry", rel)
		}
	}
}

// Every API file that frames server-sent events must be classified, so a new stream cannot ship
// without someone deciding whether its payload can carry a secret.
var sseWriters = map[string]string{
	"internal/api/deploy_attempts.go":     "masked: writeSSEEvent masks every log line",
	"internal/api/database_provision.go":  "UNREVIEWED: provisioning progress",
	"internal/api/ai_chat.go":             "UNREVIEWED: model output",
	"internal/api/v2_watch.go":            "UNREVIEWED: resource change events",
	"internal/api/domain_diagnose.go":     "UNREVIEWED: DNS diagnosis",
	"internal/api/doctor_engine.go":       "UNREVIEWED: doctor check results",
	"internal/api/go_live_checklist.go":   "UNREVIEWED: checklist results",
	"internal/api/app_run_state.go":       "structured run state only",
	"internal/api/migration_guide.go":     "UNREVIEWED: migration progress",
	"internal/api/deploy_steps.go":        "structured step names and status",
	"internal/api/deployments_stream.go":  "UNREVIEWED: deployment state events",
	"internal/api/doctor_checks_extra.go": "UNREVIEWED: doctor check results",
	"internal/api/pipelines.go":           "masked: pipeline_logs are masked at insert",
	"internal/api/deploy_timeline.go":     "UNREVIEWED: deploy timeline",
}

func TestEverySSEWriterInAPIIsClassified(t *testing.T) {
	root := repoRoot(t)
	frame := regexp.MustCompile(`data: %s`)
	seen := map[string]bool{}
	_ = filepath.WalkDir(filepath.Join(root, "internal", "api"), func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		b, _ := os.ReadFile(p) //nolint:gosec // walks this repo's own source tree
		if !frame.Match(b) {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		seen[rel] = true
		if _, ok := sseWriters[rel]; !ok {
			t.Errorf("%s writes SSE frames but is not classified in sseWriters", rel)
		}
		return nil
	})
	for rel := range sseWriters {
		if !seen[rel] {
			t.Errorf("sseWriters lists %s which no longer writes SSE frames", rel)
		}
	}
}

func TestSeededSecretNeverReachesFreeTextStoreRows(t *testing.T) {
	secretmask.Add(seededSecret)
	ctx := context.Background()
	sdb, err := store.Open(ctx, filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sdb.Close() })
	leak := "failed: token=" + seededSecret

	if err := sdb.UpsertHookRun(ctx, store.HookRun{ServiceName: "web", HookType: "pre", Command: "run " + seededSecret, Output: leak, RanAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	// The log table is only reachable through a run row; the pipeline fixture is out of scope here.
	if _, err := sdb.DB.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		t.Fatal(err)
	}
	if err := sdb.AppendPipelineLogs(ctx, []store.PipelineLogLine{{RunID: "r1", JobKey: "j", Stream: "stdout", Line: leak, CreatedAt: time.Now()}}); err != nil {
		t.Fatal(err)
	}
	if err := sdb.SaveDeployTraceStep(ctx, store.DeployTraceStep{AttemptID: "a1", Step: "build", Status: "failed", Reason: leak, Detail: leak, StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := sdb.RecordBuildRun(ctx, store.BuildRun{App: "web", Tag: "t", StartedAt: time.Now(), Status: "failed", Error: leak}); err != nil {
		t.Fatal(err)
	}
	if err := sdb.AddAppEvent(ctx, store.AppEvent{AppName: "web", Kind: "deploy", Actor: "u", Title: leak, Detail: leak}); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"service_hook_runs", "deploy_trace_steps", "build_runs", "app_events", "pipeline_logs"} {
		if rawContains(t, sdb.DB, "SELECT * FROM "+table) {
			t.Errorf("secret reached %s", table)
		}
	}
}

// Masking must not corrupt ordinary output: short common words and near misses stay intact.
func TestMaskLeavesOrdinaryTextAlone(t *testing.T) {
	r := secretmask.New()
	r.Add("canary-ordinary-secret")
	r.Add("true")
	r.Add("1234")
	for _, in := range []string{
		"server listening on port 8080, status true, id 1234",
		"canary-ordinary is a prefix only",
		"error: connection refused (retrying in 5s)",
		"",
	} {
		if got := r.Mask(in); got != in {
			t.Errorf("Mask(%q) = %q, want unchanged", in, got)
		}
	}
}
