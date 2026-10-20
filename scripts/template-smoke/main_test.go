package main

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/catalog"
)

func TestParseShard(t *testing.T) {
	tests := []struct {
		in      string
		i, n    int
		wantErr bool
	}{
		{in: "1/4", i: 1, n: 4},
		{in: "4/4", i: 4, n: 4},
		{in: "0/4", wantErr: true},
		{in: "5/4", wantErr: true},
		{in: "2", wantErr: true},
		{in: "a/b", wantErr: true},
	}
	for _, tt := range tests {
		i, n, err := parseShard(tt.in)
		if (err != nil) != tt.wantErr || (!tt.wantErr && (i != tt.i || n != tt.n)) {
			t.Errorf("parseShard(%q) = %d, %d, %v", tt.in, i, n, err)
		}
	}
}

func TestSelectTemplates(t *testing.T) {
	all := []catalog.Template{{ID: "c", Category: "Media"}, {ID: "a", Category: "media"}, {ID: "b", Category: "Dev"}, {ID: "d", Category: "Dev"}}
	ids := func(ts []catalog.Template) []string {
		var out []string
		for _, t := range ts {
			out = append(out, t.ID)
		}
		return out
	}
	tests := []struct {
		name string
		o    options
		want []string
	}{
		{name: "all, sorted", o: options{shards: 1}, want: []string{"a", "b", "c", "d"}},
		{name: "only", o: options{only: map[string]bool{"d": true, "a": true}, shards: 1}, want: []string{"a", "d"}},
		{name: "category is case-insensitive", o: options{category: "MEDIA", shards: 1}, want: []string{"a", "c"}},
		{name: "shard 1 of 2", o: options{shard: 1, shards: 2}, want: []string{"a", "c"}},
		{name: "shard 2 of 2", o: options{shard: 2, shards: 2}, want: []string{"b", "d"}},
	}
	for _, tt := range tests {
		if got := ids(selectTemplates(all, tt.o)); !slices.Equal(got, tt.want) {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestClassify(t *testing.T) {
	tests := []struct {
		name       string
		services   []serviceResult
		upErr      error
		wantStatus string
		wantIn     string
	}{
		{name: "all running", services: []serviceResult{{Name: "web", State: "running"}}, wantStatus: statusPass},
		{name: "exit 0 is a failure", services: []serviceResult{{Name: "ntfy", State: "exited", ExitCode: 0}}, wantStatus: statusFail, wantIn: "ntfy exited with code 0"},
		{name: "unhealthy", services: []serviceResult{{Name: "web", State: "running", Health: "unhealthy"}}, wantStatus: statusFail, wantIn: "healthcheck failing"},
		{name: "health starting still passes", services: []serviceResult{{Name: "web", State: "running", Health: "starting"}}, wantStatus: statusPass, wantIn: "still starting"},
		{name: "restarting", services: []serviceResult{{Name: "web", State: "restarting"}}, wantStatus: statusFail, wantIn: "web is restarting"},
		{name: "up error with missing service", services: []serviceResult{{Name: "web", State: "missing"}}, upErr: errors.New("pool exhausted"), wantStatus: statusFail, wantIn: "compose up: pool exhausted"},
	}
	for _, tt := range tests {
		got := classify(result{Services: tt.services}, tt.upErr)
		if got.Status != tt.wantStatus || !strings.Contains(got.Summary, tt.wantIn) {
			t.Errorf("%s: %s %q", tt.name, got.Status, got.Summary)
		}
	}
}

func TestOverrideAndVolumes(t *testing.T) {
	compose := "services:\n  web:\n    volumes:\n      - web_data:/data\n      - ./local:/x\n      - \"cache:/cache\"\n      - type: volume\n        source: extra\n        target: /e\n"
	vols := namedVolumes(compose)
	if !slices.Equal(vols, []string{"web_data", "cache", "extra"}) {
		t.Fatalf("namedVolumes = %v", vols)
	}
	o := overrideFor(vols, []string{"web"}, "10.231.7.0/24")
	for _, want := range []string{"ports: !reset []", "container_name: !reset null", `restart: "no"`, "subnet: 10.231.7.0/24", "  web_data: {}"} {
		if !strings.Contains(o, want) {
			t.Errorf("override missing %q:\n%s", want, o)
		}
	}
}

func TestParsePS(t *testing.T) {
	lines := `{"Service":"web","State":"running","Health":"healthy"}` + "\n" + `{"Service":"db","State":"exited","ExitCode":1}`
	got := parsePS(lines)
	if len(got) != 2 || got[1].ExitCode != 1 || got[0].Health != "healthy" {
		t.Fatalf("ndjson = %+v", got)
	}
	if arr := parsePS(`[{"Service":"web","State":"running"}]`); len(arr) != 1 {
		t.Fatalf("array = %+v", arr)
	}
}

func TestMagicValue(t *testing.T) {
	if magicValue("SERVICE_FQDN_APP") != "localhost" || magicValue("SERVICE_URL_APP") != "http://localhost" {
		t.Fatal("fqdn/url must be localhost")
	}
	if v := magicValue("SERVICE_PASSWORD_DB"); len(v) != 32 {
		t.Fatalf("password = %q", v)
	}
	if v := magicValue("SERVICE_USER_ADMIN"); !strings.HasPrefix(v, "smoke") {
		t.Fatalf("user = %q", v)
	}
}
