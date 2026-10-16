package models

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Benchmark errors, distinguishable by callers to pick an HTTP status.
var (
	ErrBenchDisabled  = errors.New("models: benchmark is not enabled on this control plane")
	ErrBenchBusy      = errors.New("models: a benchmark is already running for this model")
	ErrBenchNotReady  = errors.New("models: the model is not running; wake it first, a benchmark never starts a model")
	ErrBenchEmbedding = errors.New("models: the benchmark measures chat generation and does not apply to an embedding model")
)

// BenchRateLimitError reports when the next benchmark may run.
type BenchRateLimitError struct{ RetryAfter time.Duration }

func (e *BenchRateLimitError) Error() string {
	return fmt.Sprintf("models: benchmarks are rate limited; try again in %s", e.RetryAfter.Round(time.Second))
}

// BenchPrompt is the measurement of one fixed prompt.
type BenchPrompt struct {
	Name             string  `json:"name"`
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	TTFTMillis       int64   `json:"ttft_ms"`
	PromptTPS        float64 `json:"prompt_tokens_per_sec"`
	GenTPS           float64 `json:"gen_tokens_per_sec"`
}

// BenchResult is the stored outcome of one benchmark.
type BenchResult struct {
	Model            string        `json:"model"`
	RanAt            time.Time     `json:"ran_at"`
	DurationMillis   int64         `json:"duration_ms"`
	Prompts          []BenchPrompt `json:"prompts"`
	PromptTPS        float64       `json:"prompt_tokens_per_sec"`
	GenTPS           float64       `json:"gen_tokens_per_sec"`
	TTFTMillis       int64         `json:"ttft_ms"`
	PeakMemoryBytes  uint64        `json:"peak_memory_bytes"`
	MemoryLimitBytes uint64        `json:"memory_limit_bytes"`
	Truncated        bool          `json:"truncated"`
	Note             string        `json:"note"`
}

// BenchConfig tunes the benchmark. Zero fields take the defaults.
type BenchConfig struct {
	// Timeout is the hard cap for the whole run.
	Timeout     time.Duration
	MinInterval time.Duration
	MaxTokens   int
	SampleEvery time.Duration
}

// LoadBenchConfig reads APP_MODEL_BENCH_TIMEOUT, APP_MODEL_BENCH_MIN_INTERVAL
// and APP_MODEL_BENCH_MAX_TOKENS.
func LoadBenchConfig() BenchConfig {
	return BenchConfig{
		Timeout:     time.Duration(envInt64("APP_MODEL_BENCH_TIMEOUT_SECONDS", 120)) * time.Second,
		MinInterval: time.Duration(envInt64("APP_MODEL_BENCH_MIN_INTERVAL_SECONDS", 300)) * time.Second,
		MaxTokens:   envInt("APP_MODEL_BENCH_MAX_TOKENS", 64),
		SampleEvery: 500 * time.Millisecond,
	}
}

// BenchStatsFunc reads a running model container's memory use. ok is false
// when it cannot be read.
type BenchStatsFunc func(ctx context.Context, m store.Model) (used, limit uint64, ok bool)

type benchDeps struct {
	cfg    *BenchConfig
	client *http.Client
	stats  BenchStatsFunc
	now    func() time.Time

	mu      sync.Mutex
	last    map[string]time.Time
	running map[string]bool
}

// SetBenchmark enables the benchmark. stats may be nil (memory then reads 0).
func (s *Service) SetBenchmark(cfg BenchConfig, client *http.Client, stats BenchStatsFunc) {
	if client == nil {
		client = &http.Client{}
	}
	s.bench = benchDeps{cfg: &cfg, client: client, stats: stats, now: time.Now, last: map[string]time.Time{}, running: map[string]bool{}}
}

// BenchmarkEnabled reports whether Benchmark can run.
func (s *Service) BenchmarkEnabled() bool { return s.bench.cfg != nil }

type benchPromptDef struct{ name, text string }

var benchPrompts = []benchPromptDef{
	{"short", "Answer in one short sentence: what is a reverse proxy?"},
	{"context", strings.Repeat("The deploy finished, the container restarted twice, and the health check passed after the third attempt. ", 12) +
		"\n\nSummarize the text above in two sentences."},
	{"list", "List three common reasons a web service container restarts in a loop, one short line each."},
}

// Benchmark runs the fixed prompt set through the running model, stores the
// result and returns it. It never wakes a model.
func (s *Service) Benchmark(ctx context.Context, name string) (BenchResult, error) {
	if s.bench.cfg == nil {
		return BenchResult{}, ErrBenchDisabled
	}
	v, err := s.Get(ctx, name)
	if err != nil {
		return BenchResult{}, err
	}
	m := v.Model
	if cat, cerr := LocalCatalog(); cerr == nil {
		if e, ok := cat.FindWeights(m.WeightsRepo, m.WeightsFile); ok && e.Kind == KindEmbedding {
			return BenchResult{}, ErrBenchEmbedding
		}
	}
	if !v.Ready || m.EndpointDial == "" {
		return BenchResult{}, ErrBenchNotReady
	}
	if err := s.bench.begin(name); err != nil {
		return BenchResult{}, err
	}
	defer s.bench.end(name)

	cfg := *s.bench.cfg
	runCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	_ = s.store.TouchModel(ctx, name, s.bench.now())
	res := s.runBench(runCtx, m, cfg)
	_ = s.store.TouchModel(ctx, name, s.bench.now())
	if len(res.Prompts) == 0 {
		return BenchResult{}, fmt.Errorf("models: benchmark of %q produced no result within %s (the model may be loading or too slow)", name, cfg.Timeout)
	}
	raw, err := json.Marshal(res)
	if err != nil {
		return BenchResult{}, fmt.Errorf("models: encode benchmark of %q: %w", name, err)
	}
	if err := s.store.SetModelBenchmark(ctx, name, string(raw)); err != nil {
		return BenchResult{}, fmt.Errorf("models: store benchmark of %q: %w", name, err)
	}
	slog.Info("model benchmark finished", slog.String("model", name), slog.Float64("gen_tps", res.GenTPS), slog.Bool("truncated", res.Truncated))
	return res, nil
}

func (d *benchDeps) begin(name string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.running[name] {
		return ErrBenchBusy
	}
	if last, ok := d.last[name]; ok {
		if wait := d.cfg.MinInterval - d.now().Sub(last); wait > 0 {
			return &BenchRateLimitError{RetryAfter: wait}
		}
	}
	d.running[name] = true
	d.last[name] = d.now()
	return nil
}

func (d *benchDeps) end(name string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.running, name)
}

func (s *Service) runBench(ctx context.Context, m store.Model, cfg BenchConfig) BenchResult {
	start := s.bench.now()
	res := BenchResult{Model: m.Name, RanAt: start.UTC(), Prompts: []BenchPrompt{},
		Note: "Measured on this server just now with short fixed prompts. Real prompts and a busy server will differ."}
	stop := make(chan struct{})
	var wg sync.WaitGroup
	if s.bench.stats != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			t := time.NewTicker(cfg.SampleEvery)
			defer t.Stop()
			for {
				if used, limit, ok := s.bench.stats(ctx, m); ok {
					res.sample(used, limit)
				}
				select {
				case <-stop:
					return
				case <-ctx.Done():
					return
				case <-t.C:
				}
			}
		}()
	}
	for _, p := range benchPrompts {
		bp, err := s.benchOne(ctx, m, p, cfg.MaxTokens)
		if err != nil {
			res.Truncated = true
			slog.Warn("model benchmark prompt did not finish", slog.String("model", m.Name), slog.String("prompt", p.name), slog.String("error", err.Error()))
			break
		}
		res.Prompts = append(res.Prompts, bp)
	}
	close(stop)
	wg.Wait()
	res.DurationMillis = s.bench.now().Sub(start).Milliseconds()
	res.summarize()
	return res
}

var benchMu sync.Mutex

func (r *BenchResult) sample(used, limit uint64) {
	benchMu.Lock()
	defer benchMu.Unlock()
	r.PeakMemoryBytes = max(r.PeakMemoryBytes, used)
	r.MemoryLimitBytes = limit
}

func (r *BenchResult) summarize() {
	var pt, gt float64
	var ttfts []int64
	for _, p := range r.Prompts {
		pt += p.PromptTPS
		gt += p.GenTPS
		ttfts = append(ttfts, p.TTFTMillis)
	}
	if n := float64(len(r.Prompts)); n > 0 {
		r.PromptTPS, r.GenTPS = round1(pt/n), round1(gt/n)
		sort.Slice(ttfts, func(i, j int) bool { return ttfts[i] < ttfts[j] })
		r.TTFTMillis = ttfts[len(ttfts)/2]
	}
}

type benchChunk struct {
	Choices []struct {
		Delta struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
		} `json:"delta"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
	Timings *struct {
		PromptN         int     `json:"prompt_n"`
		PredictedN      int     `json:"predicted_n"`
		PromptPerSecond float64 `json:"prompt_per_second"`
		PredictedPerSec float64 `json:"predicted_per_second"`
	} `json:"timings"`
}

func (s *Service) benchOne(ctx context.Context, m store.Model, p benchPromptDef, maxTokens int) (BenchPrompt, error) {
	body, err := json.Marshal(map[string]any{
		"model": m.ModelRef, "stream": true, "temperature": 0, "max_tokens": maxTokens,
		"stream_options": map[string]any{"include_usage": true},
		"messages":       []map[string]string{{"role": "user", "content": p.text}},
	})
	if err != nil {
		return BenchPrompt{}, fmt.Errorf("encode prompt: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+m.EndpointDial+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return BenchPrompt{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	sent := s.bench.now()
	resp, err := s.bench.client.Do(req)
	if err != nil {
		return BenchPrompt{}, fmt.Errorf("call engine: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return BenchPrompt{}, fmt.Errorf("engine answered HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	out := BenchPrompt{Name: p.name}
	var first, last time.Time
	var timings *benchChunk
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			break
		}
		var ch benchChunk
		if json.Unmarshal([]byte(payload), &ch) != nil {
			continue
		}
		now := s.bench.now()
		if len(ch.Choices) > 0 && (ch.Choices[0].Delta.Content != "" || ch.Choices[0].Delta.ReasoningContent != "") {
			if first.IsZero() {
				first = now
			}
			last = now
		}
		if ch.Usage != nil {
			out.PromptTokens, out.CompletionTokens = ch.Usage.PromptTokens, ch.Usage.CompletionTokens
		}
		if ch.Timings != nil {
			c := ch
			timings = &c
		}
	}
	if err := sc.Err(); err != nil {
		return BenchPrompt{}, fmt.Errorf("read stream: %w", err)
	}
	if first.IsZero() {
		return BenchPrompt{}, errors.New("the engine produced no tokens")
	}
	out.TTFTMillis = first.Sub(sent).Milliseconds()
	if timings != nil {
		out.PromptTPS, out.GenTPS = round1(timings.Timings.PromptPerSecond), round1(timings.Timings.PredictedPerSec)
		if out.PromptTokens == 0 {
			out.PromptTokens = timings.Timings.PromptN
		}
		if out.CompletionTokens == 0 {
			out.CompletionTokens = timings.Timings.PredictedN
		}
		return out, nil
	}
	if ttft := first.Sub(sent).Seconds(); ttft > 0 && out.PromptTokens > 0 {
		out.PromptTPS = round1(float64(out.PromptTokens) / ttft)
	}
	if gen := last.Sub(first).Seconds(); gen > 0 && out.CompletionTokens > 1 {
		out.GenTPS = round1(float64(out.CompletionTokens-1) / gen)
	}
	return out, nil
}

// ParseBenchmark decodes a stored benchmark, nil when none ran.
func ParseBenchmark(raw string) *BenchResult {
	if raw == "" {
		return nil
	}
	var r BenchResult
	if json.Unmarshal([]byte(raw), &r) != nil {
		return nil
	}
	return &r
}
