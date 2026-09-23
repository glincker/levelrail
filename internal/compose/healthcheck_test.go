package compose

import (
	"reflect"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestResolveHealthcheck_CMDShellCurl_Translates(t *testing.T) {
	hc := &Healthcheck{
		Test:     healthcheckTest{"CMD-SHELL", "curl -f http://localhost:3000/health || exit 1"},
		Interval: "30s",
		Timeout:  "5s",
		Retries:  3,
	}
	got, warning, err := resolveHealthcheck("web", hc)
	if err != nil {
		t.Fatalf("resolveHealthcheck() error = %v", err)
	}
	if warning != "" {
		t.Errorf("warning = %q, want none", warning)
	}
	if got == nil {
		t.Fatal("got = nil, want a translated health check")
	}
	if got.Path != "/health" {
		t.Errorf("Path = %q, want /health", got.Path)
	}
	if got.Interval != 30*time.Second {
		t.Errorf("Interval = %v, want 30s", got.Interval)
	}
	if got.Timeout != 5*time.Second {
		t.Errorf("Timeout = %v, want 5s", got.Timeout)
	}
	if got.Failures != 3 {
		t.Errorf("Failures = %d, want 3", got.Failures)
	}
}

func TestResolveHealthcheck_CMDArrayCurl_Translates(t *testing.T) {
	hc := &Healthcheck{Test: healthcheckTest{"CMD", "curl", "-f", "http://127.0.0.1:8080/api/health"}}
	got, warning, err := resolveHealthcheck("api", hc)
	if err != nil {
		t.Fatalf("resolveHealthcheck() error = %v", err)
	}
	if warning != "" {
		t.Errorf("warning = %q, want none", warning)
	}
	if got == nil || got.Path != "/api/health" {
		t.Errorf("got = %+v, want Path=/api/health", got)
	}
}

func TestResolveHealthcheck_WgetSpider_Translates(t *testing.T) {
	hc := &Healthcheck{Test: healthcheckTest{"CMD-SHELL", "wget --spider -q http://localhost:80/status"}}
	got, _, err := resolveHealthcheck("web", hc)
	if err != nil {
		t.Fatalf("resolveHealthcheck() error = %v", err)
	}
	if got == nil || got.Path != "/status" {
		t.Errorf("got = %+v, want Path=/status", got)
	}
}

func TestResolveHealthcheck_URLWithNoPath_DefaultsToSlash(t *testing.T) {
	hc := &Healthcheck{Test: healthcheckTest{"CMD-SHELL", "curl -f http://localhost:3000"}}
	got, _, err := resolveHealthcheck("web", hc)
	if err != nil {
		t.Fatalf("resolveHealthcheck() error = %v", err)
	}
	if got == nil || got.Path != "/" {
		t.Errorf("got = %+v, want Path=/", got)
	}
}

func TestResolveHealthcheck_BareListWithoutPrefix_TreatedAsShellCommand(t *testing.T) {
	hc := &Healthcheck{Test: healthcheckTest{"curl", "-f", "http://localhost/health"}}
	got, warning, err := resolveHealthcheck("web", hc)
	if err != nil {
		t.Fatalf("resolveHealthcheck() error = %v", err)
	}
	if warning != "" {
		t.Errorf("warning = %q, want none", warning)
	}
	if got == nil || got.Path != "/health" {
		t.Errorf("got = %+v, want Path=/health", got)
	}
}

func TestResolveHealthcheck_TranslatesProbeShape(t *testing.T) {
	yes, no := true, false
	tests := []struct {
		name        string
		hc          Healthcheck
		want        store.ServiceProbe
		wantReady   time.Duration
		wantWarning bool
	}{
		{name: "pg_isready CMD becomes argv exec", hc: Healthcheck{Test: healthcheckTest{"CMD", "pg_isready", "-U", "postgres"}}, want: store.ServiceProbe{Exec: []string{"pg_isready", "-U", "postgres"}}},
		{name: "redis-cli CMD-SHELL becomes shell exec", hc: Healthcheck{Test: healthcheckTest{"CMD-SHELL", "redis-cli ping | grep PONG"}}, want: store.ServiceProbe{Exec: []string{"/bin/sh", "-c", "redis-cli ping | grep PONG"}}},
		{name: "mysqladmin ping", hc: Healthcheck{Test: healthcheckTest{"CMD-SHELL", "mysqladmin ping -h localhost"}}, want: store.ServiceProbe{Exec: []string{"/bin/sh", "-c", "mysqladmin ping -h localhost"}}},
		{name: "bare /dev/tcp stays unset", hc: Healthcheck{Test: healthcheckTest{"CMD-SHELL", "sh -c ': < /dev/tcp/127.0.0.1/3001' || exit 1"}}, wantWarning: true},
		{name: "curl -f without -L accepts 3xx unfollowed", hc: Healthcheck{Test: healthcheckTest{"CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:3001/"}}, want: store.ServiceProbe{Path: "/", FollowRedirects: &no, ExpectedStatus: "200-399"}},
		{name: "curl -fsSL follows", hc: Healthcheck{Test: healthcheckTest{"CMD", "curl", "-fsSL", "http://localhost/login"}}, want: store.ServiceProbe{Path: "/login", FollowRedirects: &yes, ExpectedStatus: "200-399"}},
		{name: "curl -k https", hc: Healthcheck{Test: healthcheckTest{"CMD-SHELL", "curl -fsSk https://127.0.0.1:9443/api/system/status"}}, want: store.ServiceProbe{Path: "/api/system/status", Scheme: "https", TLSSkipVerify: true, FollowRedirects: &no, ExpectedStatus: "200-399"}},
		{name: "wget https no-check-certificate keeps default follow", hc: Healthcheck{Test: healthcheckTest{"CMD-SHELL", "wget -q --no-check-certificate -O- https://localhost:8443/health"}}, want: store.ServiceProbe{Path: "/health", Scheme: "https", TLSSkipVerify: true}},
		{
			name:      "start_period widens the ready budget",
			hc:        Healthcheck{Test: healthcheckTest{"CMD-SHELL", "wget -q -O- http://127.0.0.1:80/api/_health/"}, Interval: "10s", Retries: 3, StartPeriod: "120s"},
			want:      store.ServiceProbe{Path: "/api/_health/", Interval: 10 * time.Second, Failures: 3},
			wantReady: 150 * time.Second,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hc := tt.hc
			got, warning, err := resolveHealthcheck("svc", &hc)
			if err != nil {
				t.Fatalf("resolveHealthcheck() error = %v", err)
			}
			if tt.wantWarning {
				if got != nil || warning == "" {
					t.Errorf("got = %+v, warning %q; want nil with a warning", got, warning)
				}
				return
			}
			if got == nil || warning != "" {
				t.Fatalf("got = %+v, warning %q", got, warning)
			}
			if !reflect.DeepEqual(got.ServiceProbe, tt.want) || got.ReadyTimeout != tt.wantReady {
				t.Errorf("got %+v ready %v, want %+v ready %v", got.ServiceProbe, got.ReadyTimeout, tt.want, tt.wantReady)
			}
			if err := got.ProbeConfig().Validate(); err != nil {
				t.Errorf("translated probe fails validation: %v", err)
			}
		})
	}
}

func TestResolveHealthcheck_Nil_ReturnsNilNoWarningNoError(t *testing.T) {
	got, warning, err := resolveHealthcheck("web", nil)
	if got != nil || warning != "" || err != nil {
		t.Errorf("resolveHealthcheck(nil) = (%+v, %q, %v), want (nil, \"\", nil)", got, warning, err)
	}
}

func TestResolveHealthcheck_EmptyTest_ReturnsNilNoWarningNoError(t *testing.T) {
	hc := &Healthcheck{Test: healthcheckTest{}}
	got, warning, err := resolveHealthcheck("web", hc)
	if got != nil || warning != "" || err != nil {
		t.Errorf("resolveHealthcheck(empty test) = (%+v, %q, %v), want (nil, \"\", nil)", got, warning, err)
	}
}

func TestResolveHealthcheck_None_ReturnsNilNoWarningNoError(t *testing.T) {
	hc := &Healthcheck{Test: healthcheckTest{"NONE"}}
	got, warning, err := resolveHealthcheck("web", hc)
	if got != nil || warning != "" || err != nil {
		t.Errorf("resolveHealthcheck(NONE) = (%+v, %q, %v), want (nil, \"\", nil)", got, warning, err)
	}
}

func TestResolveHealthcheck_CMDShellMissingCommand_Errors(t *testing.T) {
	hc := &Healthcheck{Test: healthcheckTest{"CMD-SHELL"}}
	if _, _, err := resolveHealthcheck("web", hc); err == nil {
		t.Fatal("resolveHealthcheck() error = nil, want an error for CMD-SHELL with no command")
	}
}

func TestResolveHealthcheck_MalformedInterval_ReturnsErrorNotPanic(t *testing.T) {
	hc := &Healthcheck{
		Test:     healthcheckTest{"CMD-SHELL", "curl -f http://localhost/health"},
		Interval: "not-a-duration",
	}
	if _, _, err := resolveHealthcheck("web", hc); err == nil {
		t.Fatal("resolveHealthcheck() error = nil, want an error for a malformed interval")
	}
}

func TestResolveHealthcheck_MalformedTimeout_ReturnsErrorNotPanic(t *testing.T) {
	hc := &Healthcheck{
		Test:    healthcheckTest{"CMD-SHELL", "curl -f http://localhost/health"},
		Timeout: "5 seconds please",
	}
	if _, _, err := resolveHealthcheck("web", hc); err == nil {
		t.Fatal("resolveHealthcheck() error = nil, want an error for a malformed timeout")
	}
}

func TestResolveHealthcheck_CombinedDuration_Parses(t *testing.T) {
	hc := &Healthcheck{
		Test:     healthcheckTest{"CMD-SHELL", "curl -f http://localhost/health"},
		Interval: "1m30s",
	}
	got, _, err := resolveHealthcheck("web", hc)
	if err != nil {
		t.Fatalf("resolveHealthcheck() error = %v", err)
	}
	if got.Interval != 90*time.Second {
		t.Errorf("Interval = %v, want 1m30s", got.Interval)
	}
}

func TestParseComposeDuration_Empty_ReturnsZero(t *testing.T) {
	d, err := parseComposeDuration("")
	if err != nil || d != 0 {
		t.Errorf("parseComposeDuration(\"\") = (%v, %v), want (0, nil)", d, err)
	}
}

func TestParseComposeDuration_Malformed_ReturnsErrorNotPanic(t *testing.T) {
	if _, err := parseComposeDuration("banana"); err == nil {
		t.Fatal("parseComposeDuration(\"banana\") error = nil, want an error")
	}
}
