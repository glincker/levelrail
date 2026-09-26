package diagnose

import (
	"strings"
	"testing"
)

func logs(s string) []string { return strings.Split(strings.TrimSpace(s), "\n") }

func TestAnalyzeTopCause(t *testing.T) {
	mem := int64(256 * mib)
	tests := []struct {
		name      string
		in        Input
		wantCode  string
		wantField string
		wantTo    string
	}{
		{"oom flag", Input{Facts: Facts{OOMKilled: true, HasExit: true, ExitCode: 137, MemoryBytes: mem}}, CauseOOMKilled, "resources.memory_bytes", itoa64(512 * mib)},
		{"oom small limit uses step", Input{Facts: Facts{OOMKilled: true, MemoryBytes: 64 * mib}}, CauseOOMKilled, "resources.memory_bytes", itoa64(320 * mib)},
		{"oom custom factor", Input{Facts: Facts{OOMKilled: true, MemoryBytes: 1024 * mib, OOMFactor: 1.5}}, CauseOOMKilled, "resources.memory_bytes", itoa64(1536 * mib)},
		{"oom exit 137 only", Input{Facts: Facts{HasExit: true, ExitCode: 137, MemoryBytes: mem}}, CauseOOMKilled, "resources.memory_bytes", itoa64(512 * mib)},
		{"oom from logs", Input{RecentLogLines: logs("Killed\nfatal: out of memory"), Facts: Facts{MemoryBytes: mem}}, CauseOOMKilled, "resources.memory_bytes", itoa64(512 * mib)},
		{"oom no limit is manual", Input{Facts: Facts{OOMKilled: true}}, CauseOOMKilled, "", ""},
		{"env not set", Input{RecentLogLines: logs("Error: DATABASE_URL is not set")}, CauseMissingEnv, "env.DATABASE_URL", ""},
		{"env required var", Input{RecentLogLines: logs("fatal: required environment variable STRIPE_KEY not provided")}, CauseMissingEnv, "env.STRIPE_KEY", ""},
		{"env node undefined", Input{RecentLogLines: logs("Error: SESSION_SECRET is undefined\n    at Object.<anonymous> (/app/server.js:3:9)")}, CauseMissingEnv, "env.SESSION_SECRET", ""},
		{"env python keyerror", Input{RecentLogLines: logs("Traceback (most recent call last):\n  File \"app.py\", line 4, in <module>\n    url = os.environ['REDIS_URL']\nKeyError: 'REDIS_URL'")}, CauseMissingEnv, "env.REDIS_URL", ""},
		{"env pydantic", Input{RecentLogLines: logs("pydantic_core._pydantic_core.ValidationError: 1 validation error for Settings\ndatabase_url\n  Field required [type=missing, input_value={}, input_type=dict]")}, CauseMissingEnv, "env.DATABASE_URL", ""},
		{"env zod", Input{RecentLogLines: logs("Invalid environment variables: { AUTH_SECRET: [ 'Required' ] }")}, CauseMissingEnv, "env.AUTH_SECRET", ""},
		{"env zod path", Input{RecentLogLines: logs("ZodError: [\n  {\n    \"code\": \"invalid_type\",\n    \"path\": [ \"API_TOKEN\" ],\n    \"message\": \"Required\"\n  }\n]")}, CauseMissingEnv, "env.API_TOKEN", ""},
		{"env go envconfig", Input{RecentLogLines: logs("envconfig.Process: required key PG_DSN missing value")}, CauseMissingEnv, "env.PG_DSN", ""},
		{"env skips configured", Input{RecentLogLines: logs("Error: DATABASE_URL is not set\nError: API_KEY is not set"), Facts: Facts{EnvKeys: []string{"DATABASE_URL"}}}, CauseMissingEnv, "env.API_KEY", ""},
		{"port in use docker", Input{Attempt: &AttemptInput{Status: "failed", Error: "Bind for 0.0.0.0:8080 failed: port is already allocated"}}, CausePortInUse, "", ""},
		{"port in use node", Input{RecentLogLines: logs("Error: listen EADDRINUSE: address already in use :::3000")}, CausePortInUse, "", ""},
		{"port in use go", Input{RecentLogLines: logs("listen tcp :8080: bind: address already in use")}, CausePortInUse, "", ""},
		{"permission node", Input{RecentLogLines: logs("Error: EACCES: permission denied, open '/data/db.sqlite'")}, CausePermissionDenied, "", ""},
		{"permission python", Input{RecentLogLines: logs("PermissionError: [Errno 13] Permission denied: '/var/lib/app/cache'")}, CausePermissionDenied, "", ""},
		{"permission mkdir", Input{RecentLogLines: logs("mkdir: cannot create directory '/data/uploads': Permission denied")}, CausePermissionDenied, "", ""},
		{"exec format", Input{RecentLogLines: logs("standard_init_linux.go:228: exec user process caused: exec format error")}, CauseExecFormatError, "", ""},
		{"platform mismatch", Input{Attempt: &AttemptInput{Status: "failed", Error: "image with reference x was found but its platform (linux/amd64) does not match the specified platform (linux/arm64); no matching manifest for linux/arm64/v8"}}, CauseExecFormatError, "", ""},
		{"cmd not found exec", Input{RecentLogLines: logs(`OCI runtime create failed: runc create failed: unable to start container process: exec: "gunicorn": executable file not found in $PATH: unknown`)}, CauseCommandNotFound, "", ""},
		{"cmd not found sh", Input{RecentLogLines: logs("/bin/sh: 1: next: not found")}, CauseCommandNotFound, "", ""},
		{"cmd exit 127", Input{Facts: Facts{HasExit: true, ExitCode: 127}}, CauseCommandNotFound, "", ""},
		{"entrypoint crlf", Input{RecentLogLines: logs("exec /usr/local/bin/docker-entrypoint.sh: no such file or directory")}, CauseCommandNotFound, "", ""},
		{"healthcheck 404", Input{Facts: Facts{HealthPath: "/health"}, Conditions: []ConditionInput{{Type: "Ready", Status: "False", Reason: "ProbeFailed", Message: "readiness probe GET /health returned status 404"}}}, CauseHealthcheckFail, "health.readiness.path", "/healthz"},
		{"healthcheck 404 skips current", Input{Facts: Facts{HealthPath: "/healthz"}, Conditions: []ConditionInput{{Type: "Ready", Status: "False", Reason: "ProbeFailed", Message: "readiness probe returned 404 Not Found"}}}, CauseHealthcheckFail, "health.readiness.path", "/health"},
		{"healthcheck refused", Input{Facts: Facts{Port: 3000}, Conditions: []ConditionInput{{Type: "Ready", Status: "False", Reason: "ProbeFailed", Message: "readiness probe: dial tcp 127.0.0.1:3000: connection refused"}}}, CauseHealthcheckFail, "", ""},
		{"healthcheck timeout", Input{Attempt: &AttemptInput{Status: "failed", Error: "timed out waiting for readiness"}}, CauseHealthcheckFail, "", ""},
		{"wrong port sockets", Input{Facts: Facts{Port: 3000, ListeningPorts: []int{8080}}}, CauseWrongPort, "port", "8080"},
		{"wrong port logs", Input{Facts: Facts{Port: 3000}, RecentLogLines: logs("Server listening on port 8000"), Conditions: []ConditionInput{{Type: "Ready", Message: "readiness probe timed out"}}}, CauseWrongPort, "port", "8000"},
		{"wrong port uvicorn", Input{Facts: Facts{Port: 3000}, RecentLogLines: logs("INFO:     Uvicorn running on http://0.0.0.0:8000 (Press CTRL+C to quit)"), Attempt: &AttemptInput{Error: "timed out waiting for readiness"}}, CauseWrongPort, "port", "8000"},
		{"wrong port vite", Input{Facts: Facts{Port: 3000}, RecentLogLines: logs("  Local:   http://localhost:5173/"), Attempt: &AttemptInput{Error: "readiness probe failed"}}, CauseWrongPort, "port", "5173"},
		{"wrong port expose", Input{Facts: Facts{Port: 3000, ExposedPorts: []int{80}}, Attempt: &AttemptInput{Error: "timed out waiting for readiness"}}, CauseWrongPort, "port", "80"},
		{"pull not found", Input{Attempt: &AttemptInput{Status: "failed", Error: "manifest for acme/web:v9 not found: manifest unknown: manifest unknown"}}, CauseImagePullFailed, "", ""},
		{"pull auth", Input{Attempt: &AttemptInput{Status: "failed", Error: "Head https://registry/v2/x/manifests/latest: unauthorized: authentication required"}}, CauseImagePullFailed, "", ""},
		{"pull rate limit", Input{Attempt: &AttemptInput{Status: "failed", Error: "toomanyrequests: You have reached your pull rate limit"}}, CauseImagePullFailed, "", ""},
		{"build out of disk", Input{Facts: Facts{BuildFailed: true}, RecentLogLines: logs("#12 ERROR: failed to copy: write /var/lib/x: no space left on device")}, CauseBuildOutOfDisk, "", ""},
		{"enospc", Input{RecentLogLines: logs("npm ERR! code ENOSPC")}, CauseBuildOutOfDisk, "", ""},
		{"crashloop generic", Input{Crashloop: &CrashloopInput{Firing: true, RestartCount: 6, RestartCountThreshold: 3, RestartWindow: "10m"}}, CauseCrashloopGeneric, "", ""},
		{"exit code generic", Input{Facts: Facts{HasExit: true, ExitCode: 1}, RecentLogLines: logs("something odd happened")}, CauseCrashloopGeneric, "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			causes := Analyze(tc.in)
			if len(causes) == 0 {
				t.Fatalf("no causes, want %s", tc.wantCode)
			}
			var got *Cause
			for i := range causes {
				if causes[i].Code == tc.wantCode {
					got = &causes[i]
					break
				}
			}
			if got == nil {
				t.Fatalf("causes %v lack %s", codes(causes), tc.wantCode)
			}
			if len(got.Evidence) == 0 || len(got.Fixes) == 0 || got.Explanation == "" {
				t.Fatalf("incomplete cause: %+v", got)
			}
			if tc.wantField == "" {
				return
			}
			for _, f := range got.Fixes {
				for _, ch := range f.Changes {
					if ch.Field == tc.wantField && (tc.wantTo == "" || ch.To == tc.wantTo) {
						return
					}
				}
			}
			t.Fatalf("no fix changing %s to %q in %+v", tc.wantField, tc.wantTo, got.Fixes)
		})
	}
}

func codes(cs []Cause) []string {
	var out []string
	for _, c := range cs {
		out = append(out, c.Code)
	}
	return out
}

func TestAnalyzeNoFalsePositives(t *testing.T) {
	tests := []struct {
		name string
		in   Input
	}{
		{"healthy logs", Input{RecentLogLines: logs("Server listening on port 3000\nGET /healthz 200")}},
		{"port matches", Input{Facts: Facts{Port: 3000, ListeningPorts: []int{3000}}}},
		{"empty", Input{}},
		{"exit zero", Input{Facts: Facts{HasExit: true, ExitCode: 0}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Analyze(tc.in); len(got) != 0 {
				t.Fatalf("unexpected causes %v", codes(got))
			}
		})
	}
}

func TestAnalyzeNumbersFixesAndRanks(t *testing.T) {
	in := Input{
		Facts:          Facts{OOMKilled: true, MemoryBytes: 128 * mib},
		RecentLogLines: logs("Error: DATABASE_URL is not set"),
	}
	causes := Analyze(in)
	if len(causes) != 2 || causes[0].Confidence != ConfidenceHigh {
		t.Fatalf("causes = %v", codes(causes))
	}
	seen := map[int]bool{}
	for _, c := range causes {
		for _, f := range c.Fixes {
			if f.N < 1 || seen[f.N] {
				t.Fatalf("bad fix number %d", f.N)
			}
			seen[f.N] = true
		}
	}
}

func TestMissingEnvRedactsEvidence(t *testing.T) {
	in := Input{RecentLogLines: logs("Error: TOKEN is not set (was Bearer abcdefghijklmnopqrstuvwxyz0123456789)")}
	for _, c := range Analyze(in) {
		for _, e := range c.Evidence {
			if strings.Contains(e.Excerpt, "abcdefghijklmnopqrstuvwxyz0123456789") {
				t.Fatalf("evidence leaked secret: %s", e.Excerpt)
			}
		}
	}
}

func TestDiagnosePromotesTypedCauseWhenLegacyUnmatched(t *testing.T) {
	res := Diagnose(Input{RecentLogLines: logs("Error: DATABASE_URL is not set")})
	if res.Confidence != ConfidenceHigh || len(res.Causes) == 0 || res.Causes[0].Code != CauseMissingEnv {
		t.Fatalf("res = %+v", res)
	}
}
