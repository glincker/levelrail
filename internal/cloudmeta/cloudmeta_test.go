package cloudmeta

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func serve(t *testing.T, h http.HandlerFunc) string {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv.URL
}

func notFound() string {
	return "http://127.0.0.1:1"
}

func cfgWith(b Bases) Config {
	return Config{Bases: b, Timeout: 500 * time.Millisecond}
}

func allDown() Bases {
	d := notFound()
	return Bases{AWS: d, GCP: d, Azure: d, Hetzner: d, DigitalOcean: d}
}

func TestDetectAWSUsesIMDSv2Token(t *testing.T) {
	var sawToken bool
	url := serve(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/latest/api/token":
			if r.Header.Get("X-aws-ec2-metadata-token-ttl-seconds") == "" {
				http.Error(w, "ttl required", http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte("tok123"))
		case r.URL.Path == "/latest/meta-data/placement/availability-zone":
			sawToken = r.Header.Get("X-aws-ec2-metadata-token") == "tok123"
			_, _ = w.Write([]byte("eu-west-1b"))
		case r.URL.Path == "/latest/meta-data/placement/region":
			_, _ = w.Write([]byte("eu-west-1"))
		case r.URL.Path == "/latest/meta-data/instance-id":
			_, _ = w.Write([]byte("i-0abc"))
		default:
			http.NotFound(w, r)
		}
	})
	b := allDown()
	b.AWS = url
	got := Detect(context.Background(), cfgWith(b))
	want := Info{Provider: "aws", Region: "eu-west-1", Zone: "eu-west-1b", InstanceID: "i-0abc", Source: SourceMetadata}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	if !sawToken {
		t.Error("zone request did not carry the IMDSv2 token")
	}
}

func TestDetectAWSDerivesRegionFromZone(t *testing.T) {
	url := serve(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/latest/api/token":
			_, _ = w.Write([]byte("t"))
		case "/latest/meta-data/placement/availability-zone":
			_, _ = w.Write([]byte("us-east-2c"))
		default:
			http.NotFound(w, r)
		}
	})
	b := allDown()
	b.AWS = url
	if got := Detect(context.Background(), cfgWith(b)); got.Region != "us-east-2" || got.Zone != "us-east-2c" {
		t.Fatalf("got %+v", got)
	}
}

func TestDetectGCPRequiresFlavorHeaderBack(t *testing.T) {
	good := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Metadata-Flavor") != "Google" {
			http.Error(w, "missing flavor", http.StatusForbidden)
			return
		}
		w.Header().Set("Metadata-Flavor", "Google")
		switch r.URL.Path {
		case "/computeMetadata/v1/instance/zone":
			_, _ = w.Write([]byte("projects/123456/zones/us-central1-a"))
		case "/computeMetadata/v1/instance/id":
			_, _ = w.Write([]byte("98765"))
		}
	})
	b := allDown()
	b.GCP = good
	got := Detect(context.Background(), cfgWith(b))
	if got.Provider != "gcp" || got.Region != "us-central1" || got.Zone != "us-central1-a" || got.InstanceID != "98765" {
		t.Fatalf("got %+v", got)
	}

	impostor := serve(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("projects/1/zones/x-y-z")) })
	b.GCP = impostor
	if got := Detect(context.Background(), cfgWith(b)); got.Found() {
		t.Fatalf("a server that does not echo Metadata-Flavor must not count as GCP, got %+v", got)
	}
}

func TestDetectAzure(t *testing.T) {
	tests := []struct {
		name, body, wantZone string
	}{
		{"with zone", `{"location":"westeurope","zone":"2","vmId":"vm-1"}`, "westeurope-2"},
		{"without zone", `{"location":"eastus","zone":"","vmId":"vm-2"}`, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			url := serve(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Metadata") != "true" || r.URL.Path != "/metadata/instance/compute" {
					http.Error(w, "bad", http.StatusBadRequest)
					return
				}
				_, _ = w.Write([]byte(tc.body))
			})
			b := allDown()
			b.Azure = url
			got := Detect(context.Background(), cfgWith(b))
			if got.Provider != "azure" || got.Zone != tc.wantZone || got.Region == "" {
				t.Fatalf("got %+v", got)
			}
		})
	}
}

func TestDetectAzureRejectsMalformedJSON(t *testing.T) {
	url := serve(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("not json")) })
	b := allDown()
	b.Azure = url
	if got := Detect(context.Background(), cfgWith(b)); got.Found() {
		t.Fatalf("got %+v", got)
	}
}

func TestDetectHetzner(t *testing.T) {
	url := serve(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/hetzner/v1/metadata/instance-id":
			_, _ = w.Write([]byte("4242\n"))
		case "/hetzner/v1/metadata/region":
			_, _ = w.Write([]byte("eu-central"))
		case "/hetzner/v1/metadata/availability-zone":
			_, _ = w.Write([]byte("fsn1-dc14"))
		default:
			http.NotFound(w, r)
		}
	})
	b := allDown()
	b.Hetzner = url
	got := Detect(context.Background(), cfgWith(b))
	want := Info{Provider: "hetzner", Region: "eu-central", Zone: "fsn1-dc14", InstanceID: "4242", Source: SourceMetadata}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestDetectDigitalOceanHasNoZone(t *testing.T) {
	url := serve(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/metadata/v1/region":
			_, _ = w.Write([]byte("nyc3"))
		case "/metadata/v1/id":
			_, _ = w.Write([]byte("555"))
		default:
			http.NotFound(w, r)
		}
	})
	b := allDown()
	b.DigitalOcean = url
	got := Detect(context.Background(), cfgWith(b))
	if got.Provider != "digitalocean" || got.Region != "nyc3" || got.Zone != "" || got.InstanceID != "555" {
		t.Fatalf("got %+v", got)
	}
}

func TestDetectNothingAnswersFallsBackToEmpty(t *testing.T) {
	if got := Detect(context.Background(), cfgWith(allDown())); got.Found() || got.Source != "" {
		t.Fatalf("got %+v", got)
	}
}

func TestDetectSlowEndpointTimesOut(t *testing.T) {
	slow := serve(t, func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(3 * time.Second):
		case <-r.Context().Done():
		}
	})
	b := allDown()
	b.AWS, b.Hetzner = slow, slow
	cfg := Config{Bases: b, Timeout: 100 * time.Millisecond}
	start := time.Now()
	if got := Detect(context.Background(), cfg); got.Found() {
		t.Fatalf("got %+v", got)
	}
	if time.Since(start) > 2*time.Second {
		t.Errorf("detection did not honor its timeout: %s", time.Since(start))
	}
}

func TestDetectFixedOrderWhenSeveralAnswer(t *testing.T) {
	both := serve(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/latest/api/token":
			_, _ = w.Write([]byte("t"))
		case "/latest/meta-data/placement/availability-zone":
			_, _ = w.Write([]byte("us-east-1a"))
		case "/metadata/v1/region":
			_, _ = w.Write([]byte("nyc1"))
		default:
			http.NotFound(w, r)
		}
	})
	b := Bases{AWS: both, GCP: notFound(), Azure: notFound(), Hetzner: notFound(), DigitalOcean: both}
	for range 5 {
		if got := Detect(context.Background(), cfgWith(b)); got.Provider != "aws" {
			t.Fatalf("got %+v, want aws first", got)
		}
	}
}

func TestOverrideBeatsMetadataAndFillsGaps(t *testing.T) {
	url := serve(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/metadata/v1/region":
			_, _ = w.Write([]byte("nyc3"))
		default:
			http.NotFound(w, r)
		}
	})
	b := allDown()
	b.DigitalOcean = url
	cfg := cfgWith(b)
	cfg.Override = Info{Zone: "rack-4"}
	got := Detect(context.Background(), cfg)
	if got.Provider != "digitalocean" || got.Region != "nyc3" || got.Zone != "rack-4" || got.Source != SourceEnv {
		t.Fatalf("got %+v", got)
	}
}

func TestOverrideAloneWorksOnAHomeServer(t *testing.T) {
	cfg := cfgWith(allDown())
	cfg.Override = Info{Provider: "home", Region: "basement"}
	got := Detect(context.Background(), cfg)
	if got.Provider != "home" || got.Region != "basement" || got.Source != SourceEnv {
		t.Fatalf("got %+v", got)
	}
}

func TestDisabledSkipsProbingButKeepsOverride(t *testing.T) {
	hit := false
	url := serve(t, func(http.ResponseWriter, *http.Request) { hit = true })
	b := allDown()
	b.AWS, b.Hetzner = url, url
	cfg := cfgWith(b)
	cfg.Disabled = true
	cfg.Override = Info{Provider: "other"}
	got := Detect(context.Background(), cfg)
	if hit {
		t.Error("disabled detection still called a metadata endpoint")
	}
	if got.Provider != "other" {
		t.Fatalf("got %+v", got)
	}
}

func TestFromEnv(t *testing.T) {
	env := map[string]string{
		EnvTimeout: "250ms", EnvDisable: "true",
		EnvProvider: "Hetzner Cloud", EnvRegion: "eu central", EnvZone: "  ",
	}
	cfg := FromEnv(func(k string) (string, bool) { v, ok := env[k]; return v, ok })
	if cfg.Timeout != 250*time.Millisecond || !cfg.Disabled {
		t.Errorf("cfg = %+v", cfg)
	}
	if cfg.Override.Provider != "Hetzner-Cloud" || cfg.Override.Region != "eu-central" || cfg.Override.Zone != "" {
		t.Errorf("override = %+v", cfg.Override)
	}
	def := FromEnv(func(string) (string, bool) { return "", false })
	if def.Timeout != time.Second || def.Disabled {
		t.Errorf("defaults = %+v", def)
	}
}

func TestLabelValue(t *testing.T) {
	long := ""
	for range 80 {
		long += "a"
	}
	tests := map[string]string{
		"us-east-1a":   "us-east-1a",
		"  eu west 1 ": "eu-west-1",
		"--x--":        "x",
		"":             "",
		"!!!":          "",
		long:           long[:63],
	}
	for in, want := range tests {
		if got := LabelValue(in); got != want {
			t.Errorf("LabelValue(%q) = %q, want %q", in, got, want)
		}
	}
}
