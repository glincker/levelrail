package observability

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/glincker/levelrail/internal/store"
)

const secretValue = "s3cret-value-xyz"

var sampleFields = map[string]map[string]string{
	"grafana-cloud": {"endpoint": "https://otlp-gateway-prod-eu-west-0.grafana.net/otlp/", "instance_id": "4242", "token": secretValue},
	"datadog":       {"site": "datadoghq.eu", "api_key": secretValue},
	"grafana-loki":  {"endpoint": "https://logs-prod-012.grafana.net/", "user": "777", "token": secretValue},
	"axiom":         {"dataset": "lr", "region": "eu", "token": secretValue},
	"honeycomb":     {"region": "eu", "dataset": "lr", "api_key": secretValue},
	"better-stack":  {"ingesting_host": "s1.eu-nbg-2.betterstackdata.com", "source_token": secretValue},
	"otlp-http":     {"endpoint": "https://collector.example.com", "header_value": "Bearer " + secretValue},
}

func TestCatalogRendersEveryPresetAndSignal(t *testing.T) {
	t.Parallel()
	for _, p := range Catalog() {
		fields, ok := sampleFields[p.ID]
		if !ok {
			t.Fatalf("no sample fields for preset %s", p.ID)
		}
		got, err := p.Render("prod", fields, p.SignalNames())
		if err != nil {
			t.Fatalf("%s: %v", p.ID, err)
		}
		if len(got) != len(p.Signals) {
			t.Fatalf("%s: rendered %d of %d signals", p.ID, len(got), len(p.Signals))
		}
		for _, r := range got {
			if !strings.HasPrefix(r.Endpoint, "https://") || strings.Contains(r.Endpoint, "{") {
				t.Errorf("%s/%s endpoint = %q", p.ID, r.Signal, r.Endpoint)
			}
			if strings.Contains(r.Endpoint, secretValue) || strings.Contains(strings.Join(r.HeaderNames, ","), secretValue) {
				t.Errorf("%s/%s preview leaks the secret", p.ID, r.Signal)
			}
			if r.Input.Name != DestinationName("prod", p.ID, r.Signal) {
				t.Errorf("%s/%s name = %q", p.ID, r.Signal, r.Input.Name)
			}
			if r.Input.APIKey == "" && len(r.Input.Headers) == 0 {
				t.Errorf("%s/%s carries no credential", p.ID, r.Signal)
			}
		}
	}
}

func TestRenderProviderSpecifics(t *testing.T) {
	t.Parallel()
	grafana, _ := Lookup("grafana-cloud")
	got, err := grafana.Render("g", sampleFields["grafana-cloud"], nil)
	if err != nil || len(got) != 2 {
		t.Fatalf("grafana = %v, %v", got, err)
	}
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("4242:"+secretValue))
	if got[0].Endpoint != "https://otlp-gateway-prod-eu-west-0.grafana.net/otlp/v1/logs" || got[0].Input.Headers["Authorization"] != want {
		t.Fatalf("grafana logs = %q %v", got[0].Endpoint, got[0].HeaderNames)
	}

	dd, _ := Lookup("datadog")
	got, err = dd.Render("d", sampleFields["datadog"], nil)
	if err != nil || len(got) != 1 || got[0].Input.Kind != store.ExportKindLogsDatadog || got[0].Endpoint != "https://http-intake.logs.datadoghq.eu/api/v2/logs" {
		t.Fatalf("datadog default = %+v, %v", got, err)
	}
	got, err = dd.Render("d", sampleFields["datadog"], []string{"metrics"})
	if err != nil || got[0].Endpoint != "https://otlp.datadoghq.eu/v1/metrics" || got[0].Input.Headers["dd-api-key"] != secretValue {
		t.Fatalf("datadog metrics = %+v, %v", got, err)
	}

	hc, _ := Lookup("honeycomb")
	got, _ = hc.Render("h", sampleFields["honeycomb"], []string{"logs"})
	if got[0].Endpoint != "https://api.eu1.honeycomb.io/v1/logs" || got[0].Input.Headers["x-honeycomb-team"] != secretValue {
		t.Fatalf("honeycomb = %+v", got[0])
	}

	ax, _ := Lookup("axiom")
	got, _ = ax.Render("a", sampleFields["axiom"], []string{"logs", "metrics"})
	if got[0].Endpoint != "https://eu-central-1.aws.edge.axiom.co/v1/ingest/lr" || got[1].Endpoint != "https://eu-central-1.aws.edge.axiom.co/v1/metrics" {
		t.Fatalf("axiom = %q %q", got[0].Endpoint, got[1].Endpoint)
	}

	if got[1].Input.Headers["x-axiom-metrics-dataset"] != "lr" || got[1].Input.Headers["X-Axiom-Dataset"] != "" {
		t.Fatalf("axiom metrics dataset header = %v", got[1].Input.Headers)
	}
	usAx := map[string]string{"dataset": "lr", "token": secretValue}
	got, _ = ax.Render("a", usAx, []string{"metrics"})
	if got[0].Endpoint != "https://us-east-1.aws.edge.axiom.co/v1/metrics" {
		t.Fatalf("axiom us metrics = %q", got[0].Endpoint)
	}
	got, _ = ax.Render("a", sampleFields["axiom"], []string{"metrics"})
	if got[0].Endpoint != "https://eu-central-1.aws.edge.axiom.co/v1/metrics" {
		t.Fatalf("axiom eu metrics = %q", got[0].Endpoint)
	}

	got, _ = dd.Render("d", map[string]string{"site": "uk1.datadoghq.com", "api_key": secretValue}, []string{"metrics"})
	if _, bad := got[0].Input.Headers["dd-protocol"]; bad || got[0].Endpoint != "https://otlp.uk1.datadoghq.com/v1/metrics" {
		t.Fatalf("datadog uk1 metrics = %+v", got[0])
	}
	if _, err := dd.Render("d", map[string]string{"site": "ap2.datadoghq.com", "api_key": secretValue}, nil); err != nil {
		t.Fatalf("datadog ap2: %v", err)
	}

	hcNoDS, _ := hc.Render("h", map[string]string{"api_key": secretValue}, nil)
	for _, r := range hcNoDS {
		if _, has := r.Input.Headers["x-honeycomb-dataset"]; has || len(r.Input.Headers) != 1 {
			t.Fatalf("honeycomb without dataset must send only the team header, got %v", r.HeaderNames)
		}
	}
	got, _ = hc.Render("h", sampleFields["honeycomb"], []string{"logs"})
	if got[0].Input.Headers["x-honeycomb-dataset"] != "lr" {
		t.Fatalf("honeycomb classic dataset header missing: %v", got[0].HeaderNames)
	}

	bs, _ := Lookup("better-stack")
	got, err = bs.Render("b", sampleFields["better-stack"], nil)
	if err != nil || got[0].Endpoint != "https://s1.eu-nbg-2.betterstackdata.com" || got[0].Input.APIKey != secretValue {
		t.Fatalf("better stack = %+v, %v", got, err)
	}
	if _, err := bs.Render("b", map[string]string{"source_token": "t"}, nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("better stack without host: %v", err)
	}

	open, _ := Lookup("otlp-http")
	got, _ = open.Render("o", map[string]string{"endpoint": "https://collector.example.com"}, []string{"logs"})
	if len(got[0].Input.Headers) != 0 {
		t.Fatalf("blank credential must send no header, got %v", got[0].HeaderNames)
	}
}

func TestGrafanaLokiPreset(t *testing.T) {
	t.Parallel()
	p, ok := Lookup("grafana-loki")
	if !ok {
		t.Fatal("grafana-loki preset missing")
	}
	got, err := p.Render("l", sampleFields["grafana-loki"], nil)
	if err != nil || len(got) != 1 || got[0].Input.Kind != store.ExportKindLogsLoki {
		t.Fatalf("loki = %+v, %v", got, err)
	}
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("777:"+secretValue))
	if got[0].Endpoint != "https://logs-prod-012.grafana.net" || got[0].Input.Headers["Authorization"] != want {
		t.Fatalf("loki render = %q %v", got[0].Endpoint, got[0].HeaderNames)
	}
	if _, has := got[0].Input.Headers["X-Scope-OrgID"]; has {
		t.Fatal("blank tenant must send no tenant header")
	}
	got, _ = p.Render("l", map[string]string{"endpoint": "https://loki.example.com", "tenant": "team-a"}, nil)
	if len(got[0].Input.Headers) != 1 || got[0].Input.Headers["X-Scope-OrgID"] != "team-a" {
		t.Fatalf("tenant only headers = %v", got[0].Input.Headers)
	}
	if _, err := p.Render("l", map[string]string{"user": "1"}, nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing endpoint: %v", err)
	}
}

func TestRenderValidation(t *testing.T) {
	t.Parallel()
	grafana, _ := Lookup("grafana-cloud")
	hc, _ := Lookup("honeycomb")
	for name, tc := range map[string]struct {
		p       Preset
		fields  map[string]string
		signals []string
	}{
		"missing required": {grafana, map[string]string{"endpoint": "https://x.grafana.net/otlp"}, nil},
		"unknown field":    {grafana, map[string]string{"endpoint": "https://x", "instance_id": "1", "token": "t", "extra": "1"}, nil},
		"bad choice":       {hc, map[string]string{"region": "mars", "dataset": "d", "api_key": "k"}, nil},
		"unknown signal":   {hc, map[string]string{"dataset": "d", "api_key": "k"}, []string{"traces"}},
		"newline value":    {grafana, map[string]string{"endpoint": "https://x\nHost: evil", "instance_id": "1", "token": "t"}, nil},
	} {
		if _, err := tc.p.Render("n", tc.fields, tc.signals); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
}

func TestHintMapsStatusToProviderText(t *testing.T) {
	t.Parallel()
	p, _ := Lookup("axiom")
	for code, wantSub := range map[int]string{401: "rejected the token", 403: "ingest", 404: "dataset", 429: "rate limiting", 503: "server error", 422: "rejected the request"} {
		status, hint := p.Hint(fmt.Errorf("otlp logs returned status %d: nope", code))
		if status != code || !strings.Contains(hint, wantSub) {
			t.Errorf("status %d: got %d %q", code, status, hint)
		}
	}
	if status, hint := p.Hint(errors.New("dial tcp: no such host")); status != 0 || hint != "" {
		t.Errorf("network error should carry no status hint, got %d %q", status, hint)
	}
}

func TestProviderHints(t *testing.T) {
	t.Parallel()
	for id, tc := range map[string]map[int]string{
		"better-stack": {402: "quota", 403: "source token", 406: "MessagePack", 413: "10 MiB"},
		"otlp-http":    {400: "malformed", 413: "larger", 429: "rate limiting", 502: "retried", 503: "retried", 504: "retried"},
		"datadog":      {400: "delta", 413: "5.1 MiB"},
	} {
		p, _ := Lookup(id)
		for code, sub := range tc {
			if _, h := p.Hint(fmt.Errorf("export returned status %d", code)); !strings.Contains(h, sub) {
				t.Errorf("%s %d hint = %q, want %q", id, code, h, sub)
			}
		}
	}
}

func TestDestinationNameRoundTrip(t *testing.T) {
	t.Parallel()
	e, p, s, ok := ParseDestinationName(DestinationName("prod", "axiom", "logs"))
	if !ok || e != "prod" || p != "axiom" || s != "logs" {
		t.Fatalf("round trip = %q %q %q %v", e, p, s, ok)
	}
	if _, _, _, ok := ParseDestinationName("manual-loki"); ok {
		t.Fatal("a hand made destination must not parse")
	}
}
