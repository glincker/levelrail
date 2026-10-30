package objectstore

import "testing"

func TestResolve_Garage(t *testing.T) {
	got, err := Resolve(PresetGarage, "http://10.0.0.5:3900/", "", "")
	if err != nil {
		t.Fatalf("Resolve garage: %v", err)
	}
	if got.Endpoint != "http://10.0.0.5:3900" || got.Region != "garage" {
		t.Fatalf("Resolve garage = %+v, want trimmed endpoint and region garage", got)
	}
	if _, err := Resolve(PresetGarage, "", "", ""); err == nil {
		t.Fatal("Resolve garage without endpoint succeeded, want error")
	}
	if p, ok := PresetByID(PresetGarage); !ok || !p.PathStyle || !p.NeedsEndpoint {
		t.Fatalf("garage preset = %+v, %v", p, ok)
	}
}

func TestEgressPricePerGB(t *testing.T) {
	env := map[string]string{
		EgressPricePrefix + "AWS": "0.05",
		EgressPricePrefix + "R2":  "-1",
		EgressPricePrefix + "B2":  "nope",
	}
	lookup := func(k string) (string, bool) { v, ok := env[k]; return v, ok }
	tests := []struct {
		name   string
		preset string
		lookup func(string) (string, bool)
		want   float64
	}{
		{"aws default", PresetAWS, nil, 0.09},
		{"aws override", PresetAWS, lookup, 0.05},
		{"negative ignored", PresetR2, lookup, 0},
		{"unparsable ignored", PresetB2, lookup, 0.01},
		{"garage free", PresetGarage, nil, 0},
		{"minio free", PresetMinIO, nil, 0},
		{"unknown free", "mystery", nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := EgressPricePerGB(tt.preset, tt.lookup); got != tt.want {
				t.Fatalf("EgressPricePerGB(%q) = %v, want %v", tt.preset, got, tt.want)
			}
		})
	}
}
