package plugin

import (
	"context"
	"slices"
	"strings"
	"testing"
)

const fromConfigExtra = "outbound_hosts_from_config: [server_url]\n"

func TestConfigHostsManifestFieldValidation(t *testing.T) {
	cases := []struct {
		name    string
		extra   string
		wantErr string
	}{
		{"valid", fromConfigExtra, ""},
		{"bad name", "outbound_hosts_from_config: ['Server URL']\n", "not a valid config field name"},
		{"too many", "outbound_hosts_from_config: [a, b, c, d, e, f, g, h, i]\n", "at most"},
	}
	for _, c := range cases {
		_, err := ParseManifest(manifestV("1.0.0", testDigest, "[read]", "[]", c.extra))
		if (c.wantErr == "") != (err == nil) || (err != nil && !strings.Contains(err.Error(), c.wantErr)) {
			t.Errorf("%s: err = %v, want %q", c.name, err, c.wantErr)
		}
	}
}

func TestEffectiveHostsFromConfigValues(t *testing.T) {
	man, err := ParseManifest(manifestV("1.0.0", testDigest, "[read]", "[api.example.com]", fromConfigExtra))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, cfg string
		want      []string
		wantErr   bool
	}{
		{"url with port", `{"server_url":"https://Chat.Example.org:8448/path"}`, []string{"api.example.com", "chat.example.org:8448"}, false},
		{"bare host", `{"server_url":"chat.example.org"}`, []string{"api.example.com", "chat.example.org"}, false},
		{"unset", `{}`, []string{"api.example.com"}, false},
		{"bad scheme", `{"server_url":"ftp://chat.example.org"}`, nil, true},
		{"bad host", `{"server_url":"https://bad host"}`, nil, true},
		{"bad port", `{"server_url":"chat.example.org:99999"}`, nil, true},
		{"not a string", `{"server_url":5}`, nil, true},
	}
	for _, c := range cases {
		got, err := EffectiveOutboundHosts(man, c.cfg)
		if (err != nil) != c.wantErr || (!c.wantErr && !slices.Equal(got, c.want)) {
			t.Errorf("%s: got %v, %v; want %v (err %v)", c.name, got, err, c.want, c.wantErr)
		}
	}
}

func TestConfigHostChangeReopensApproval(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	v1 := h.request(manifestV("1.0.0", testDigest, "[read]", "[]", fromConfigExtra))
	v1.ConfigJSON = `{"server_url":"https://one.example.org"}`
	if _, err := h.m.Install(ctx, v1, "admin"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.m.Approve(ctx, "example", []string{"read"}, "admin"); err != nil {
		t.Fatal(err)
	}
	same := h.requestV2(manifestV("1.1.0", testDigest2, "[read]", "[]", fromConfigExtra))
	pv, err := h.m.Inspect(ctx, same)
	if err != nil || pv.ReapprovalRequired {
		t.Fatalf("same host preview = %+v, %v; want no re-approval", pv, err)
	}
	moved := h.requestV2(manifestV("1.1.0", testDigest2, "[read]", "[]", fromConfigExtra))
	moved.ConfigJSON = `{"server_url":"https://two.example.org:8443"}`
	pv, err = h.m.Inspect(ctx, moved)
	if err != nil || !pv.ReapprovalRequired || !slices.Equal(pv.ConfigHosts, []string{"two.example.org:8443"}) {
		t.Fatalf("moved host preview = %+v, %v; want re-approval flagged from config", pv, err)
	}
	rec, err := h.m.Upgrade(ctx, moved, "admin")
	if err != nil || rec.ApprovedAt != "" || !slices.Equal(rec.OutboundHosts, []string{"two.example.org:8443"}) {
		t.Fatalf("moved host upgrade = %+v, %v; want approval cleared and new host recorded", rec, err)
	}
}
