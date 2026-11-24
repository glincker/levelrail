package lognorm

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
)

// Wildcard marks a token position that differed across lines of one cluster.
const Wildcard = "<*>"

// DefaultMaxClusters is the per-stream cap until the logs.miner_max_clusters
// runtime setting is wired (needs design: the miner has no caller yet).
const DefaultMaxClusters = 200

// DefaultSimilarity is the share of equal tokens a line needs to join a cluster.
const DefaultSimilarity = 0.5

// MinerConfig bounds one stream's miner. Zero values fall back to the defaults.
type MinerConfig struct {
	MaxClusters int
	Similarity  float64
}

// MinedTemplate is one mined cluster: a stable id, how many lines it absorbed and
// a masked sample line.
type MinedTemplate struct {
	ID       string
	Template string
	Count    int
	Sample   string
}

type cluster struct {
	tokens []string
	count  int
	sample string
}

// Miner is a small Drain-style clusterer: lines group by token count and first
// token, then merge with the closest cluster above the similarity bar. It is
// not safe for concurrent use; one miner per stream.
type Miner struct {
	cfg      MinerConfig
	buckets  map[string][]*cluster
	clusters int
	overflow *cluster
}

// NewMiner returns an empty miner.
func NewMiner(cfg MinerConfig) *Miner {
	if cfg.MaxClusters <= 0 {
		cfg.MaxClusters = DefaultMaxClusters
	}
	if cfg.Similarity <= 0 || cfg.Similarity > 1 {
		cfg.Similarity = DefaultSimilarity
	}
	return &Miner{cfg: cfg, buckets: map[string][]*cluster{}}
}

// Add feeds one normalized entry. The entry sample is already secret-masked,
// and the template carries placeholders only, so nothing raw is retained.
func (m *Miner) Add(e Entry) {
	toks := strings.Fields(e.Template)
	if len(toks) == 0 {
		return
	}
	key := bucketKey(toks)
	best, bestSim := (*cluster)(nil), -1.0
	for _, c := range m.buckets[key] {
		if s := similarity(c.tokens, toks); s > bestSim {
			best, bestSim = c, s
		}
	}
	if best != nil && bestSim >= m.cfg.Similarity {
		merge(best, toks)
		best.count++
		return
	}
	if m.clusters >= m.cfg.MaxClusters {
		// At the cap new shapes fold into one catch-all so counts still add up.
		if m.overflow == nil {
			m.overflow = &cluster{tokens: []string{Wildcard}, sample: e.Sample}
		}
		m.overflow.count++
		return
	}
	m.clusters++
	m.buckets[key] = append(m.buckets[key], &cluster{tokens: toks, count: 1, sample: e.Sample})
}

// AddAll feeds a batch in order.
func (m *Miner) AddAll(entries []Entry) {
	for _, e := range entries {
		m.Add(e)
	}
}

// Templates returns clusters by descending count, then template text.
func (m *Miner) Templates() []MinedTemplate {
	out := make([]MinedTemplate, 0, m.clusters+1)
	for _, cs := range m.buckets {
		for _, c := range cs {
			out = append(out, toTemplate(c))
		}
	}
	if m.overflow != nil {
		out = append(out, toTemplate(m.overflow))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Template < out[j].Template
	})
	return out
}

func toTemplate(c *cluster) MinedTemplate {
	text := strings.Join(c.tokens, " ")
	sum := sha256.Sum256([]byte(text))
	return MinedTemplate{ID: hex.EncodeToString(sum[:6]), Template: text, Count: c.count, Sample: c.sample}
}

// bucketKey pairs token count with the first token unless it is variable.
func bucketKey(toks []string) string {
	first := toks[0]
	if strings.Contains(first, "<") {
		first = Wildcard
	}
	return strconv.Itoa(len(toks)) + "|" + first
}

func similarity(tpl, toks []string) float64 {
	same := 0
	for i := range tpl {
		if tpl[i] == toks[i] && tpl[i] != Wildcard {
			same++
		}
	}
	return float64(same) / float64(len(tpl))
}

func merge(c *cluster, toks []string) {
	for i := range c.tokens {
		if c.tokens[i] != toks[i] {
			c.tokens[i] = Wildcard
		}
	}
}
