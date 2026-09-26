package models

import (
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Fit verdicts. FitUnknown means the node facts needed to judge are missing.
const (
	FitFits    = "fits"
	FitTight   = "tight"
	FitWontFit = "wont_fit"
	FitUnknown = "unknown"
)

// FitNote states the limits of every fit number.
const FitNote = "Fit figures are estimates: weights plus a flat overhead for the KV cache and compute buffers. " +
	"Real usage depends on context length, batch size, engine and driver."

var (
	quantTokenRe = regexp.MustCompile(`(?i)(?:^|[^A-Za-z0-9])(?:UD-)?(I?Q[1-8](?:_[A-Z0-9]+)*|BF16|F16|F32|MXFP4)(?:[^A-Za-z0-9]|$)`)
)

// Quant is one GGUF quantization of a repository: all shards of one
// variant summed.
type Quant struct {
	Name        string   `json:"name"`
	Bytes       int64    `json:"bytes"`
	Files       []string `json:"files"`
	Fit         string   `json:"fit"`
	Recommended bool     `json:"recommended"`
}

// FitConfig holds the estimation knobs.
type FitConfig struct {
	// OverheadPercent is added to weight bytes for KV cache and buffers.
	OverheadPercent float64
	// FitPercent is the share of free VRAM a model may use to count as
	// "fits"; above it (up to 100%) it is "tight".
	FitPercent float64
	// DiskHeadroomPercent is extra free disk required beyond the download.
	DiskHeadroomPercent float64
}

// LoadFitConfig reads APP_MODEL_FIT_OVERHEAD_PERCENT, APP_MODEL_FIT_PERCENT
// and APP_MODEL_DISK_HEADROOM_PERCENT.
func LoadFitConfig() FitConfig {
	return FitConfig{
		OverheadPercent:     envPercent("APP_MODEL_FIT_OVERHEAD_PERCENT", 20),
		FitPercent:          envPercent("APP_MODEL_FIT_PERCENT", 90),
		DiskHeadroomPercent: envPercent("APP_MODEL_DISK_HEADROOM_PERCENT", 10),
	}
}

func envPercent(key string, def float64) float64 {
	if v, err := strconv.ParseFloat(os.Getenv(key), 64); err == nil && v >= 0 {
		return v
	}
	return def
}

// QuantName extracts the quantization label from a GGUF file path, or ""
// when there is none or the file is not a weight file.
func QuantName(path string) string {
	lower := strings.ToLower(path)
	if !strings.HasSuffix(lower, ".gguf") || strings.Contains(lower, "mmproj") {
		return ""
	}
	m := quantTokenRe.FindStringSubmatch(path)
	if m == nil {
		return ""
	}
	return strings.ToUpper(m[1])
}

// DetectQuants groups GGUF files by quantization, smallest first. Split
// shards of one variant are summed.
func DetectQuants(files []HFFile) []Quant {
	byName := map[string]*Quant{}
	for _, f := range files {
		name := QuantName(f.Name)
		if name == "" {
			continue
		}
		q := byName[name]
		if q == nil {
			q = &Quant{Name: name, Fit: FitUnknown}
			byName[name] = q
		}
		q.Bytes += f.Bytes
		q.Files = append(q.Files, f.Name)
	}
	out := make([]Quant, 0, len(byName))
	for _, q := range byName {
		sort.Strings(q.Files)
		out = append(out, *q)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Bytes != out[j].Bytes {
			return out[i].Bytes < out[j].Bytes
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// FitVerdict judges weightBytes against free VRAM. freeVRAMBytes < 0 means
// unknown.
func (c FitConfig) FitVerdict(weightBytes, freeVRAMBytes int64) string {
	if freeVRAMBytes < 0 || weightBytes <= 0 {
		return FitUnknown
	}
	need := float64(weightBytes) * (1 + c.OverheadPercent/100)
	free := float64(freeVRAMBytes)
	switch {
	case need <= free*c.FitPercent/100:
		return FitFits
	case need <= free:
		return FitTight
	}
	return FitWontFit
}

// DiskOK reports whether downloading bytes leaves the configured headroom.
// freeDisk < 0 means unknown, which counts as OK (the caller reports it).
func (c FitConfig) DiskOK(bytes, freeDisk int64) bool {
	if freeDisk < 0 {
		return true
	}
	return float64(bytes)*(1+c.DiskHeadroomPercent/100) <= float64(freeDisk)
}

var (
	fullPrecision     = map[string]bool{"F32": true, "F16": true, "BF16": true}
	defaultQuantOrder = []string{"Q4_K_M", "Q4_K_S", "Q5_K_M", "Q4_0", "Q5_K_S", "Q8_0"}
)

// MarkFits sets Fit on every quant and Recommended on the best one, and
// returns the recommended name with a one-line reason. It picks the
// largest quantized variant that fits VRAM and disk, then the largest that
// is tight. With no VRAM facts it falls back to the conventional default.
func (c FitConfig) MarkFits(quants []Quant, freeVRAMBytes, freeDiskBytes int64) (name, reason string) {
	for i := range quants {
		quants[i].Fit = c.FitVerdict(quants[i].Bytes, freeVRAMBytes)
	}
	if len(quants) == 0 {
		return "", ""
	}
	pool := make([]int, 0, len(quants))
	for i, q := range quants {
		if !fullPrecision[q.Name] {
			pool = append(pool, i)
		}
	}
	if len(pool) == 0 {
		for i := range quants {
			pool = append(pool, i)
		}
	}
	pick := -1
	if freeVRAMBytes < 0 {
		pick = c.defaultPick(quants, pool, freeDiskBytes)
		if pick >= 0 {
			reason = "conventional default; this node's free VRAM is unknown"
		}
	} else {
		for _, want := range []string{FitFits, FitTight} {
			for _, i := range pool {
				if quants[i].Fit == want && c.DiskOK(quants[i].Bytes, freeDiskBytes) && (pick < 0 || quants[i].Bytes > quants[pick].Bytes) {
					pick = i
				}
			}
			if pick >= 0 {
				reason = "largest quantization that " + map[string]string{FitFits: "fits", FitTight: "just fits"}[want] + " free VRAM (estimate)"
				break
			}
		}
	}
	if pick < 0 {
		return "", ""
	}
	quants[pick].Recommended = true
	return quants[pick].Name, reason
}

func (c FitConfig) defaultPick(quants []Quant, pool []int, freeDisk int64) int {
	for _, want := range defaultQuantOrder {
		for _, i := range pool {
			if quants[i].Name == want && c.DiskOK(quants[i].Bytes, freeDisk) {
				return i
			}
		}
	}
	for _, i := range pool {
		if c.DiskOK(quants[i].Bytes, freeDisk) {
			return i
		}
	}
	return -1
}
