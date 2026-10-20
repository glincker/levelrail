package models

import "testing"

func TestQuantName(t *testing.T) {
	tests := []struct{ file, want string }{
		{"Llama-3.2-3B-Instruct-Q4_K_M.gguf", "Q4_K_M"},
		{"Llama-3.2-3B-Instruct-IQ4_XS.gguf", "IQ4_XS"},
		{"model.Q8_0.gguf", "Q8_0"},
		{"Q4_K_M/model-00001-of-00002.gguf", "Q4_K_M"},
		{"Qwen3-30B-A3B-UD-Q4_K_XL.gguf", "Q4_K_XL"},
		{"gemma-3-27b-it-BF16.gguf", "BF16"},
		{"llama-f16.gguf", "F16"},
		{"gpt-oss-20b-MXFP4.gguf", "MXFP4"},
		{"mmproj-model-f16.gguf", ""},
		{"README.md", ""},
		{"model.safetensors", ""},
		{"Llama-3.2-3B-Instruct.gguf", ""},
		{"Model-Q4_K_M.bin", ""},
	}
	for _, tt := range tests {
		if got := QuantName(tt.file); got != tt.want {
			t.Errorf("QuantName(%q) = %q, want %q", tt.file, got, tt.want)
		}
	}
}

func TestDetectQuants_SumsSplitShardsAndSorts(t *testing.T) {
	files := []HFFile{
		{"m-Q8_0-00001-of-00002.gguf", 5 * gib},
		{"m-Q8_0-00002-of-00002.gguf", 4 * gib},
		{"m-Q2_K.gguf", gib},
		{"mmproj-f16.gguf", gib / 2},
		{"README.md", 10},
	}
	got := DetectQuants(files)
	if len(got) != 2 || got[0].Name != "Q2_K" || got[1].Name != "Q8_0" || got[1].Bytes != 9*gib || len(got[1].Files) != 2 {
		t.Errorf("DetectQuants = %+v", got)
	}
}

func TestFitVerdict(t *testing.T) {
	c := FitConfig{OverheadPercent: 20, FitPercent: 90, DiskHeadroomPercent: 10}
	tests := []struct {
		name          string
		weights, vram int64
		want          string
	}{
		{"comfortable", 4 * gib, 16 * gib, FitFits},
		{"exactly at fit line", 9 * gib, 12 * gib, FitFits},
		{"just over fit line is tight", 9*gib + gib/2, 12 * gib, FitTight},
		{"exactly full is tight", 10 * gib, 12 * gib, FitTight},
		{"over is no", 11 * gib, 12 * gib, FitWontFit},
		{"unknown vram", 4 * gib, -1, FitUnknown},
		{"zero vram free is no", 4 * gib, 0, FitWontFit},
		{"unknown size", 0, 16 * gib, FitUnknown},
	}
	for _, tt := range tests {
		if got := c.FitVerdict(tt.weights, tt.vram); got != tt.want {
			t.Errorf("%s: FitVerdict(%d,%d) = %q, want %q", tt.name, tt.weights, tt.vram, got, tt.want)
		}
	}
}

func TestMarkFits_RecommendationTable(t *testing.T) {
	c := FitConfig{OverheadPercent: 20, FitPercent: 90, DiskHeadroomPercent: 10}
	base := func() []Quant {
		return []Quant{
			{Name: "Q2_K", Bytes: 2 * gib}, {Name: "Q4_K_M", Bytes: 4 * gib}, {Name: "Q5_K_M", Bytes: 5 * gib},
			{Name: "Q8_0", Bytes: 8 * gib}, {Name: "F16", Bytes: 16 * gib},
		}
	}
	tests := []struct {
		name     string
		quants   []Quant
		vram     int64
		disk     int64
		want     string
		wantFits map[string]string
	}{
		{name: "20 GiB takes q8 not f16", quants: base(), vram: 20 * gib, disk: 100 * gib, want: "Q8_0",
			wantFits: map[string]string{"Q8_0": FitFits, "F16": FitTight}},
		{name: "8 GiB takes q5", quants: base(), vram: 8 * gib, disk: 100 * gib, want: "Q5_K_M",
			wantFits: map[string]string{"Q5_K_M": FitFits, "Q8_0": FitWontFit}},
		{name: "tight only", quants: []Quant{{Name: "Q4_K_M", Bytes: 5 * gib}}, vram: 6 * gib, disk: 100 * gib, want: "Q4_K_M",
			wantFits: map[string]string{"Q4_K_M": FitTight}},
		{name: "nothing fits", quants: base(), vram: gib, disk: 100 * gib, want: ""},
		{name: "disk excludes the biggest that fits", quants: base(), vram: 24 * gib, disk: 4*gib + gib/2 + gib/10, want: "Q4_K_M"},
		{name: "disk excludes everything", quants: base(), vram: 24 * gib, disk: gib, want: ""},
		{name: "unknown vram uses conventional default", quants: base(), vram: -1, disk: 100 * gib, want: "Q4_K_M",
			wantFits: map[string]string{"Q4_K_M": FitUnknown}},
		{name: "unknown vram default respects disk", quants: base(), vram: -1, disk: 3 * gib, want: "Q2_K"},
		{name: "unknown everything", quants: base(), vram: -1, disk: -1, want: "Q4_K_M"},
		{name: "only full precision still recommendable", quants: []Quant{{Name: "F16", Bytes: 2 * gib}}, vram: 24 * gib, disk: -1, want: "F16"},
		{name: "no quants", quants: nil, vram: 24 * gib, disk: -1, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := c.MarkFits(tt.quants, tt.vram, tt.disk)
			if got != tt.want {
				t.Fatalf("recommended %q, want %q (%+v)", got, tt.want, tt.quants)
			}
			flagged := 0
			for _, q := range tt.quants {
				if q.Recommended {
					flagged++
					if q.Name != tt.want {
						t.Errorf("wrong quant flagged: %s", q.Name)
					}
				}
				if want, ok := tt.wantFits[q.Name]; ok && q.Fit != want {
					t.Errorf("%s fit = %q, want %q", q.Name, q.Fit, want)
				}
			}
			if (tt.want != "") != (flagged == 1) {
				t.Errorf("flagged %d quants for want %q", flagged, tt.want)
			}
		})
	}
}

func TestLoadFitConfig_EnvOverrides(t *testing.T) {
	t.Setenv("APP_MODEL_FIT_OVERHEAD_PERCENT", "50")
	t.Setenv("APP_MODEL_FIT_PERCENT", "80")
	t.Setenv("APP_MODEL_DISK_HEADROOM_PERCENT", "junk")
	c := LoadFitConfig()
	if c.OverheadPercent != 50 || c.FitPercent != 80 || c.DiskHeadroomPercent != 10 {
		t.Errorf("config = %+v", c)
	}
}
