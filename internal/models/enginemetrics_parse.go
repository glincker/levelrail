package models

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Sample metric names written for a model's engine under resource
// "model:<name>".
const (
	MetricEngineKVCachePercent = "engine_kv_cache_percent"
	MetricEngineRunning        = "engine_requests_running"
	MetricEngineWaiting        = "engine_requests_waiting"
	MetricEnginePrefixHitPct   = "engine_prefix_cache_hit_percent"
	MetricEngineGenRate        = "engine_tokens_per_second"
	MetricEngineTTFTSeconds    = "engine_ttft_seconds"
	MetricEngineVRAMBytes      = "engine_vram_bytes"
	MetricEngineOffloadPercent = "engine_offload_percent"
)

const maxMetricsBody = 4 << 20

// engineScrape holds the raw readings of one scrape. Nil means the engine
// did not report that value. Counter fields are cumulative.
type engineScrape struct {
	kvPercent      *float64
	running        *float64
	waiting        *float64
	prefixHits     *float64
	prefixQueries  *float64
	genTokens      *float64
	ttftSum        *float64
	ttftCount      *float64
	vramBytes      *float64
	offloadPercent *float64
}

// promSeries maps a metric name to the sum of its samples across labels.
type promSeries map[string]float64

func parseProm(r io.Reader) (promSeries, error) {
	out := promSeries{}
	sc := bufio.NewScanner(io.LimitReader(r, maxMetricsBody))
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, rest := line, ""
		if i := strings.IndexAny(line, "{ "); i >= 0 {
			name = line[:i]
			rest = line[i:]
		}
		if j := strings.LastIndex(rest, "}"); j >= 0 {
			rest = rest[j+1:]
		}
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			continue
		}
		v, err := strconv.ParseFloat(fields[0], 64)
		if err != nil {
			continue
		}
		out[name] += v
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("models: read metrics: %w", err)
	}
	return out, nil
}

func (p promSeries) first(names ...string) *float64 {
	for _, n := range names {
		if v, ok := p[n]; ok {
			return &v
		}
	}
	return nil
}

func scaled(v *float64, factor float64) *float64 {
	if v == nil {
		return nil
	}
	s := *v * factor
	return &s
}

func scrapeFromVLLM(p promSeries) engineScrape {
	return engineScrape{
		kvPercent:     scaled(p.first("vllm:kv_cache_usage_perc", "vllm:gpu_cache_usage_perc"), 100),
		running:       p.first("vllm:num_requests_running"),
		waiting:       p.first("vllm:num_requests_waiting"),
		prefixHits:    p.first("vllm:prefix_cache_hits_total", "vllm:gpu_prefix_cache_hits_total"),
		prefixQueries: p.first("vllm:prefix_cache_queries_total", "vllm:gpu_prefix_cache_queries_total"),
		genTokens:     p.first("vllm:generation_tokens_total"),
		ttftSum:       p.first("vllm:time_to_first_token_seconds_sum"),
		ttftCount:     p.first("vllm:time_to_first_token_seconds_count"),
	}
}

func scrapeFromLlamaCpp(p promSeries) engineScrape {
	return engineScrape{
		kvPercent: scaled(p.first("llamacpp:kv_cache_usage_ratio"), 100),
		running:   p.first("llamacpp:requests_processing"),
		waiting:   p.first("llamacpp:requests_deferred"),
		genTokens: p.first("llamacpp:tokens_predicted_total"),
	}
}

type ollamaPS struct {
	Models []struct {
		Size     int64 `json:"size"`
		SizeVRAM int64 `json:"size_vram"`
	} `json:"models"`
}

func scrapeFromOllamaPS(r io.Reader) (engineScrape, error) {
	var ps ollamaPS
	if err := json.NewDecoder(io.LimitReader(r, maxMetricsBody)).Decode(&ps); err != nil {
		return engineScrape{}, fmt.Errorf("models: decode ollama ps: %w", err)
	}
	if len(ps.Models) == 0 {
		return engineScrape{}, nil
	}
	var size, vram int64
	for _, m := range ps.Models {
		size += m.Size
		vram += m.SizeVRAM
	}
	v := float64(vram)
	s := engineScrape{vramBytes: &v}
	if size > 0 {
		off := float64(size-vram) / float64(size) * 100
		s.offloadPercent = &off
	}
	return s, nil
}

// engineRates are interval values computed from two cumulative scrapes.
// Counter resets (an engine restart) yield no rate rather than a negative one.
type engineRates struct {
	tokensPerSecond *float64
	prefixHitPct    *float64
	ttftSeconds     *float64
}

func computeRates(prev, cur engineScrape, seconds float64) engineRates {
	var r engineRates
	if seconds <= 0 {
		return r
	}
	if d, ok := delta(prev.genTokens, cur.genTokens); ok {
		v := d / seconds
		r.tokensPerSecond = &v
	}
	if q, ok := delta(prev.prefixQueries, cur.prefixQueries); ok && q > 0 {
		if h, ok := delta(prev.prefixHits, cur.prefixHits); ok {
			v := h / q * 100
			r.prefixHitPct = &v
		}
	}
	if c, ok := delta(prev.ttftCount, cur.ttftCount); ok && c > 0 {
		if s, ok := delta(prev.ttftSum, cur.ttftSum); ok {
			v := s / c
			r.ttftSeconds = &v
		}
	}
	return r
}

func delta(prev, cur *float64) (float64, bool) {
	if prev == nil || cur == nil || *cur < *prev {
		return 0, false
	}
	return *cur - *prev, true
}
