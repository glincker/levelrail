package compose

import (
	"strings"
	"testing"
	"time"
)

func platformProbeServices(t *testing.T, doc string) ([]string, error) {
	t.Helper()
	f, err := Parse([]byte(doc))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	svcs, warnings, err := ToDesiredServices("app", f)
	if err != nil {
		return nil, err
	}
	if len(svcs) != 1 || svcs[0].Health == nil || svcs[0].Health.Readiness == nil {
		t.Fatalf("want one service with readiness, got %+v", svcs)
	}
	r := svcs[0].Health.Readiness
	if !r.Platform || len(r.Exec) != 0 {
		t.Fatalf("readiness must be a platform probe with no exec, got %+v", r)
	}
	return warnings, nil
}

func TestPlatformProbeHTTPReplacesExecHealthcheck(t *testing.T) {
	doc := `
services:
  web:
    image: example/scratch:1
    ports: ["8080"]
    healthcheck:
      test: ["CMD", "/app", "health"]
    x-levelrail-healthcheck:
      kind: http
      path: /health
      port: 8080
      interval: 5s
      timeout: 2s
      retries: 3
      start_period: 10s
      expect_status: 200-299
`
	f, err := Parse([]byte(doc))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	svcs, _, err := ToDesiredServices("app", f)
	if err != nil {
		t.Fatalf("ToDesiredServices: %v", err)
	}
	h := svcs[0].Health
	r := h.Readiness
	if !r.Platform || r.TCP || len(r.Exec) != 0 || r.Path != "/health" || r.ExpectedStatus != "200-299" {
		t.Fatalf("unexpected probe %+v", r)
	}
	if r.FollowRedirects == nil || *r.FollowRedirects {
		t.Fatalf("platform probe must not follow redirects, got %v", r.FollowRedirects)
	}
	if r.Interval != 5*time.Second || r.Timeout != 2*time.Second || r.Failures != 3 {
		t.Fatalf("durations not carried: %+v", r)
	}
	if h.ReadyTimeout != 25*time.Second {
		t.Fatalf("ReadyTimeout = %s, want 25s", h.ReadyTimeout)
	}
	if r.Kind() != "http" {
		t.Fatalf("Kind = %q", r.Kind())
	}
}

func TestPlatformProbeTCP(t *testing.T) {
	_, err := platformProbeServices(t, `
services:
  db:
    image: example/distroless:1
    ports: ["5432"]
    x-levelrail-healthcheck:
      kind: tcp
`)
	if err != nil {
		t.Fatalf("ToDesiredServices: %v", err)
	}
}

func TestPlatformProbeRejectsBadInput(t *testing.T) {
	cases := map[string]string{
		"unpublished port": `
services:
  web:
    image: x:1
    ports: ["8080"]
    x-levelrail-healthcheck: {kind: http, port: 9090}
`,
		"no port": `
services:
  web:
    image: x:1
    x-levelrail-healthcheck: {kind: tcp}
`,
		"bad kind": `
services:
  web:
    image: x:1
    ports: ["8080"]
    x-levelrail-healthcheck: {kind: grpc}
`,
		"tcp with path": `
services:
  web:
    image: x:1
    ports: ["8080"]
    x-levelrail-healthcheck: {kind: tcp, path: /x}
`,
		"bad status": `
services:
  web:
    image: x:1
    ports: ["8080"]
    x-levelrail-healthcheck: {kind: http, expect_status: nope}
`,
	}
	for name, doc := range cases {
		t.Run(name, func(t *testing.T) {
			f, err := Parse([]byte(doc))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			_, _, err = ToDesiredServices("app", f)
			if err == nil || !strings.Contains(err.Error(), "x-levelrail-healthcheck") {
				t.Fatalf("want an x-levelrail-healthcheck error, got %v", err)
			}
		})
	}
}
