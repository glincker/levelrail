package models

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

const vllmMetricsBody = `# HELP vllm:kv_cache_usage_perc KV cache usage
# TYPE vllm:kv_cache_usage_perc gauge
vllm:kv_cache_usage_perc{engine="0",model_name="m"} 0.42
vllm:num_requests_running{engine="0"} 3
vllm:num_requests_waiting{engine="0"} 1
vllm:prefix_cache_hits_total{engine="0"} %HITS%
vllm:prefix_cache_queries_total{engine="0"} %QUERIES%
vllm:generation_tokens_total{engine="0"} %TOKENS%
vllm:time_to_first_token_seconds_sum{engine="0"} %TTFTSUM%
vllm:time_to_first_token_seconds_count{engine="0"} %TTFTCOUNT%
`

func fptr(v float64) *float64 { return &v }

func TestParsePromSumsLabelsAndSkipsComments(t *testing.T) {
	p, err := parseProm(strings.NewReader("# c\nfoo{a=\"1\"} 2\nfoo{a=\"2\"} 3.5\nbar 7\nbad line\nbaz{x=\"}\"} 1e3\n"))
	if err != nil {
		t.Fatal(err)
	}
	if p["foo"] != 5.5 || p["bar"] != 7 || p["baz"] != 1000 {
		t.Fatalf("parsed = %v", p)
	}
}

func TestScrapeMappings(t *testing.T) {
	t.Run("vllm v1 names and kv scaled to percent", func(t *testing.T) {
		p, _ := parseProm(strings.NewReader(strings.NewReplacer("%HITS%", "5", "%QUERIES%", "10", "%TOKENS%", "100", "%TTFTSUM%", "2", "%TTFTCOUNT%", "4").Replace(vllmMetricsBody)))
		s := scrapeFromVLLM(p)
		if s.kvPercent == nil || *s.kvPercent != 42 || *s.running != 3 || *s.waiting != 1 {
			t.Fatalf("scrape = %+v", s)
		}
	})
	t.Run("vllm legacy kv name", func(t *testing.T) {
		p, _ := parseProm(strings.NewReader("vllm:gpu_cache_usage_perc 0.5\n"))
		if s := scrapeFromVLLM(p); s.kvPercent == nil || *s.kvPercent != 50 {
			t.Fatalf("scrape = %+v", s)
		}
	})
	t.Run("llamacpp has no prefix or ttft", func(t *testing.T) {
		p, _ := parseProm(strings.NewReader("llamacpp:kv_cache_usage_ratio 0.25\nllamacpp:requests_processing 2\nllamacpp:requests_deferred 0\nllamacpp:tokens_predicted_total 9\n"))
		s := scrapeFromLlamaCpp(p)
		if *s.kvPercent != 25 || *s.running != 2 || *s.waiting != 0 || s.prefixHits != nil || s.ttftSum != nil {
			t.Fatalf("scrape = %+v", s)
		}
	})
	t.Run("ollama ps reports vram and offload", func(t *testing.T) {
		s, err := scrapeFromOllamaPS(strings.NewReader(`{"models":[{"size":1000,"size_vram":750}]}`))
		if err != nil || *s.vramBytes != 750 || *s.offloadPercent != 25 {
			t.Fatalf("scrape = %+v err=%v", s, err)
		}
	})
	t.Run("ollama ps with nothing loaded has no readings", func(t *testing.T) {
		s, err := scrapeFromOllamaPS(strings.NewReader(`{"models":[]}`))
		if err != nil || s.vramBytes != nil {
			t.Fatalf("scrape = %+v err=%v", s, err)
		}
	})
}

func TestComputeRates(t *testing.T) {
	tests := []struct {
		name       string
		prev, cur  engineScrape
		seconds    float64
		wantTPS    *float64
		wantPrefix *float64
		wantTTFT   *float64
	}{
		{
			name:    "all three from deltas",
			prev:    engineScrape{genTokens: fptr(100), prefixHits: fptr(5), prefixQueries: fptr(10), ttftSum: fptr(2), ttftCount: fptr(4)},
			cur:     engineScrape{genTokens: fptr(400), prefixHits: fptr(15), prefixQueries: fptr(30), ttftSum: fptr(3), ttftCount: fptr(6)},
			seconds: 15, wantTPS: fptr(20), wantPrefix: fptr(50), wantTTFT: fptr(0.5),
		},
		{
			name:    "counter reset yields no rate",
			prev:    engineScrape{genTokens: fptr(400)},
			cur:     engineScrape{genTokens: fptr(10)},
			seconds: 15,
		},
		{
			name:    "no new queries means no prefix rate",
			prev:    engineScrape{prefixHits: fptr(1), prefixQueries: fptr(2)},
			cur:     engineScrape{prefixHits: fptr(1), prefixQueries: fptr(2)},
			seconds: 15,
		},
		{name: "zero interval", prev: engineScrape{genTokens: fptr(1)}, cur: engineScrape{genTokens: fptr(9)}, seconds: 0},
	}
	eq := func(a, b *float64) bool { return (a == nil) == (b == nil) && (a == nil || *a == *b) }
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := computeRates(tc.prev, tc.cur, tc.seconds)
			if !eq(r.tokensPerSecond, tc.wantTPS) || !eq(r.prefixHitPct, tc.wantPrefix) || !eq(r.ttftSeconds, tc.wantTTFT) {
				t.Fatalf("rates = %+v", r)
			}
		})
	}
}

type fakeMetricsStore struct{ models []store.Model }

func (f fakeMetricsStore) ListModels(context.Context) ([]store.Model, error) { return f.models, nil }

type memSink struct {
	mu      sync.Mutex
	samples []telemetry.Sample
}

func (m *memSink) WriteSamples(_ context.Context, s []telemetry.Sample) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.samples = append(m.samples, s...)
	return nil
}

func (m *memSink) latest(metric string) (float64, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := len(m.samples) - 1; i >= 0; i-- {
		if m.samples[i].Metric == metric {
			return m.samples[i].Value, true
		}
	}
	return 0, false
}

func TestCollectorWritesGaugesThenRates(t *testing.T) {
	var mu sync.Mutex
	tokens := "100"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		body := strings.NewReplacer("%HITS%", "0", "%QUERIES%", "0", "%TOKENS%", tokens, "%TTFTSUM%", "0", "%TTFTCOUNT%", "0").Replace(vllmMetricsBody)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	dial := strings.TrimPrefix(srv.URL, "http://")
	sink := &memSink{}
	c := NewEngineMetricsCollector(fakeMetricsStore{models: []store.Model{
		{Name: "chat", Engine: EngineVLLM, EndpointDial: dial},
		{Name: "gone", Engine: EngineVLLM, EndpointDial: dial, Deleting: true},
		{Name: "down", Engine: EngineVLLM},
	}}, sink, srv.Client(), nil)

	t0 := time.Unix(1_700_000_000, 0)
	c.Collect(context.Background(), t0)
	if v, ok := sink.latest(MetricEngineKVCachePercent); !ok || v != 42 {
		t.Fatalf("kv sample = %v %v", v, ok)
	}
	if _, ok := sink.latest(MetricEngineGenRate); ok {
		t.Fatal("first scrape must not produce a rate")
	}
	mu.Lock()
	tokens = "400"
	mu.Unlock()
	c.Collect(context.Background(), t0.Add(15*time.Second))
	if v, ok := sink.latest(MetricEngineGenRate); !ok || v != 20 {
		t.Fatalf("tokens/s = %v %v", v, ok)
	}
	for _, s := range sink.samples {
		if s.ResourceID != "model:chat" {
			t.Fatalf("unexpected resource %q: deleting and unreachable models are skipped", s.ResourceID)
		}
	}
}

func TestCollectorSkipsUnreachableEngineWithoutWriting(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "metrics disabled", http.StatusNotImplemented)
	}))
	defer srv.Close()
	sink := &memSink{}
	c := NewEngineMetricsCollector(fakeMetricsStore{models: []store.Model{
		{Name: "cpp", Engine: EngineLlamaCpp, EndpointDial: strings.TrimPrefix(srv.URL, "http://")},
	}}, sink, srv.Client(), nil)
	c.Collect(context.Background(), time.Now())
	if len(sink.samples) != 0 {
		t.Fatalf("samples = %v", sink.samples)
	}
}

type fakeMetricsReader map[string][]telemetry.Sample

func (f fakeMetricsReader) Query(_ context.Context, resource, metric string, _, _ time.Time) ([]telemetry.Sample, error) {
	return f[resource+"/"+metric], nil
}

func TestEngineMetricsReportSupportAndHealth(t *testing.T) {
	now := time.Now()
	sample := func(v float64) []telemetry.Sample { return []telemetry.Sample{{Timestamp: now, Value: v}} }
	tests := []struct {
		name      string
		engine    string
		data      fakeMetricsReader
		wantState string
		wantUnsup []string
	}{
		{name: "vllm healthy", engine: EngineVLLM, data: fakeMetricsReader{"model:m/" + MetricEngineKVCachePercent: sample(40)}, wantState: EngineHealthOK, wantUnsup: []string{"vram", "cpu_offload"}},
		{name: "vllm kv nearly full warns", engine: EngineVLLM, data: fakeMetricsReader{"model:m/" + MetricEngineKVCachePercent: sample(97)}, wantState: EngineHealthWarn, wantUnsup: []string{"vram", "cpu_offload"}},
		{name: "vllm queue warns", engine: EngineVLLM, data: fakeMetricsReader{"model:m/" + MetricEngineWaiting: sample(12)}, wantState: EngineHealthWarn, wantUnsup: []string{"vram", "cpu_offload"}},
		{name: "no samples is unknown", engine: EngineLlamaCpp, data: fakeMetricsReader{}, wantState: EngineHealthUnknown, wantUnsup: []string{"prefix_hits", "ttft", "vram", "cpu_offload"}},
		{name: "ollama offload warns and hides kv", engine: EngineOllama, data: fakeMetricsReader{"model:m/" + MetricEngineOffloadPercent: sample(30)}, wantState: EngineHealthWarn, wantUnsup: []string{"kv_cache", "queue", "running", "prefix_hits", "tokens_per_second", "ttft"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := newSvc(t, nil)
			s.SetEngineMetricsReader(tc.data)
			ref := map[string]string{EngineOllama: "llama3.1:8b", EngineVLLM: "org/model", EngineLlamaCpp: "org/model-GGUF"}[tc.engine]
			if _, err := s.Create(context.Background(), CreateInput{Spec: Spec{Name: "m", Engine: tc.engine, ModelRef: ref}}); err != nil {
				t.Fatal(err)
			}
			rep, err := s.EngineMetrics(context.Background(), "m", time.Hour)
			if err != nil {
				t.Fatal(err)
			}
			if rep.Health.State != tc.wantState {
				t.Errorf("health = %+v, want %s", rep.Health, tc.wantState)
			}
			var got []string
			for _, ser := range rep.Series {
				if !ser.Supported {
					got = append(got, ser.ID)
				}
			}
			if strings.Join(got, ",") != strings.Join(tc.wantUnsup, ",") {
				t.Errorf("unsupported = %v, want %v", got, tc.wantUnsup)
			}
			if rep.Note == "" {
				t.Error("note must explain engine limits")
			}
		})
	}
}
