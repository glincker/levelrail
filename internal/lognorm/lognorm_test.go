package lognorm

import (
	"strings"
	"testing"

	"github.com/glincker/levelrail/internal/secretmask"
)

func TestExtract(t *testing.T) {
	cases := []struct{ name, in, msg, level, err string }{
		{"pino", `{"level":"error","time":1700000000,"msg":"database connection refused","err":"dial tcp 10.0.0.5:5432: connect: connection refused"}`, "database connection refused", "error", "dial tcp 10.0.0.5:5432: connect: connection refused"},
		{"slog", `{"time":"2026-11-24T10:00:01Z","level":"ERROR","msg":"healthcheck failed","error":{"message":"GET /healthz returned 503"}}`, "healthcheck failed", "error", "GET /healthz returned 503"},
		{"plain", `Error: listen EADDRINUSE: address already in use :::3000`, "Error: listen EADDRINUSE: address already in use :::3000", "", ""},
		{"unknown keys", `{"foo":"bar"}`, `{"foo":"bar"}`, "", ""},
		{"bad json", `{not json}`, `{not json}`, "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := Extract(c.in)
			if r.Message != c.msg || r.Level != c.level || r.Error != c.err {
				t.Fatalf("got %+v", r)
			}
		})
	}
	if got := Extract(`{"msg":"boot failed","error":"boot failed"}`).Text(); got != "boot failed" {
		t.Fatalf("duplicate error text: %q", got)
	}
}

func TestJoin(t *testing.T) {
	goPanic := []string{
		"2026/11/24 10:00:01 starting api",
		"panic: runtime error: invalid memory address or nil pointer dereference",
		"[signal SIGSEGV: segmentation violation code=0x1 addr=0x0 pc=0x4a1b2c]",
		"",
		"goroutine 1 [running]:",
		"main.(*Server).Start(0x0)",
		"\t/app/server.go:42 +0x1f",
		"main.main()",
		"\t/app/main.go:12 +0x25",
		"exit status 2",
		"next line",
	}
	java := []string{
		"Exception in thread \"main\" java.lang.OutOfMemoryError: Java heap space",
		"\tat java.base/java.util.Arrays.copyOf(Arrays.java:3512)",
		"\tat com.acme.Loader.load(Loader.java:88)",
		"Caused by: java.io.IOException: disk full",
		"\t... 3 more",
		"INFO done",
	}
	py := []string{
		"Traceback (most recent call last):",
		`  File "/app/main.py", line 10, in <module>`,
		"    connect()",
		"psycopg2.OperationalError: could not connect to server",
		"after",
	}
	node := []string{
		"Error: listen EADDRINUSE: address already in use :::3000",
		"    at Server.setupListenHandle [as _listen2] (node:net:1740:16)",
		"    at listenInCluster (node:net:1788:12)",
		"ready",
	}
	cases := []struct {
		name string
		in   []string
		want int
		head string
		last string
	}{
		{"go", goPanic, 3, "panic: runtime", "exit status 2"},
		{"java", java, 2, "Exception in thread", "\t... 3 more"},
		{"python", py, 2, "Traceback", "psycopg2.OperationalError: could not connect to server"},
		{"node", node, 2, "Error: listen EADDRINUSE", "    at listenInCluster (node:net:1788:12)"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Join(c.in)
			if len(got) != c.want {
				t.Fatalf("records %d want %d: %q", len(got), c.want, got)
			}
			idx := 0
			if c.name == "go" {
				idx = 1
			}
			if !strings.HasPrefix(got[idx], c.head) || !strings.HasSuffix(got[idx], c.last) {
				t.Fatalf("bad record %q", got[idx])
			}
		})
	}
}

func TestJoinCaps(t *testing.T) {
	in := []string{"Exception in thread \"main\" java.lang.StackOverflowError"}
	for i := 0; i < 500; i++ {
		in = append(in, "\tat com.acme.Rec.call(Rec.java:1)")
	}
	got := Join(in)
	if len(got) != 1 || strings.Count(got[0], "\n") > MaxStackLines || !strings.Contains(got[0], truncatedMarker) {
		t.Fatalf("not capped: %d lines", strings.Count(got[0], "\n"))
	}
}

func TestPlaceholders(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"crash", "2026-11-24T10:00:01.123Z worker 7 exited with code 139 pid 48213", "<TS> worker <NUM> exited with code <NUM> pid <NUM>"},
		{"oom", "Out of memory: Killed process 4021 (node) total-vm:2048000kB at 10:01:02", "Out of memory: Killed process <NUM> (node) total-vm:<NUM>kB at <TS>"},
		{"port", "listen tcp 0.0.0.0:8080: bind: address already in use", "listen tcp <IP>: bind: address already in use"},
		{"env", "missing required env DATABASE_URL (container 3f2a9c1b7d4e)", "missing required env DATABASE_URL (container <HEX>)"},
		{"health", "healthcheck failed for 9b2f4c1e-3a7d-4e1f-8c2a-5d6e7f8a9b0c: GET http://172.18.0.4:3000/healthz 503 after 3 retries", "healthcheck failed for <UUID>: GET http://<IP>/healthz <NUM> after <NUM> retries"},
		{"syslog", "Nov 24 10:00:01 host kernel: oom", "<TS> host kernel: oom"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Placeholders(c.in); got != c.want {
				t.Fatalf("got %q", got)
			}
		})
	}
}

func TestNormalizeMasksSecrets(t *testing.T) {
	reg := secretmask.New()
	reg.Add("hunter2-super-secret")
	lines := []string{
		`{"level":"error","msg":"auth failed token=hunter2-super-secret port 5432"}`,
		"plain line with hunter2-super-secret inside",
	}
	got := Normalize(lines, reg)
	if len(got) != 2 {
		t.Fatalf("entries %d", len(got))
	}
	for _, e := range got {
		if strings.Contains(e.Sample, "hunter2") || strings.Contains(e.Template, "hunter2") {
			t.Fatalf("secret leaked: %+v", e)
		}
	}
	if got[0].Level != "error" || got[0].Template != "auth failed token=[redacted] port <NUM>" {
		t.Fatalf("got %+v", got[0])
	}
}

func TestScrubPatterns(t *testing.T) {
	jwt := "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0In0.c2lnbmF0dXJl"
	cases := []struct{ name, in, want string }{
		{"jwt", "token " + jwt + " rejected", "token [redacted] rejected"},
		{"bearer", "Authorization: Bearer abc.def-123", "Authorization: Bearer [redacted]"},
		{"basic", "Authorization: Basic dXNlcjpwYXNz", "Authorization: Basic [redacted]"},
		{"url creds", "dial postgres://admin:hunter2@db:5432/app failed", "dial postgres://[redacted]@db:5432/app failed"},
		{"pem", "key -----BEGIN RSA PRIVATE KEY-----\nMIIabc\n-----END RSA PRIVATE KEY----- done", "key [redacted] done"},
		{"query", "GET /x?a=1&access_token=zzz&X-Amz-Signature=abc123&password=p%40ss&secret=s HTTP/1.1", "GET /x?a=1&access_token=[redacted]&X-Amz-Signature=[redacted]&password=[redacted]&secret=[redacted] HTTP/1.1"},
		{"plain", "listening on port 3000 for user admin", "listening on port 3000 for user admin"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Sample(c.in, nil); got != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
			if got := Scrub(c.in); got != c.want {
				t.Fatalf("Scrub got %q, want %q", got, c.want)
			}
		})
	}
}
