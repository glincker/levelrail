package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/edgefw"
)

func TestParseWhen(t *testing.T) {
	tests := []struct {
		in      string
		want    edgefw.Condition
		wantErr bool
	}{
		{"path starts_with /admin", edgefw.Condition{Field: "path", Op: "starts_with", Value: "/admin"}, false},
		{"header:X-Env equals prod", edgefw.Condition{Field: "header", Name: "X-Env", Op: "equals", Value: "prod"}, false},
		{"query:debug exists", edgefw.Condition{Field: "query", Name: "debug", Op: "exists"}, false},
		{"user_agent contains bad bot", edgefw.Condition{Field: "user_agent", Op: "contains", Value: "bad bot"}, false},
		{"ip in_cidr 10.0.0.0/8", edgefw.Condition{Field: "ip", Op: "in_cidr", Value: "10.0.0.0/8"}, false},
		{"path", edgefw.Condition{}, true},
		{"color equals red", edgefw.Condition{}, true},
	}
	for _, tt := range tests {
		got, err := parseWhen(tt.in)
		if (err != nil) != tt.wantErr || got != tt.want {
			t.Errorf("parseWhen(%q) = %+v, %v", tt.in, got, err)
		}
	}
}

func TestMoveID(t *testing.T) {
	mk := func(ids ...string) []apiclient.FirewallEdgeRule {
		var out []apiclient.FirewallEdgeRule
		for _, id := range ids {
			out = append(out, apiclient.FirewallEdgeRule{Rule: edgefw.Rule{ID: id}})
		}
		return out
	}
	tests := []struct {
		id      string
		to      int
		want    string
		wantErr bool
	}{
		{"c", 1, "c,a,b", false}, {"a", 3, "b,c,a", false}, {"b", 2, "a,b,c", false},
		{"a", 0, "", true}, {"a", 4, "", true}, {"zzz", 1, "", true},
	}
	for _, tt := range tests {
		got, err := moveID(mk("a", "b", "c"), tt.id, tt.to)
		if (err != nil) != tt.wantErr || strings.Join(got, ",") != tt.want {
			t.Errorf("moveID(%s,%d) = %v, %v", tt.id, tt.to, got, err)
		}
	}
}

func TestRun_FirewallIngress_RulesBlockBansAttack(t *testing.T) {
	var calls []string
	var created edgefw.Rule
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.RequestURI())
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/firewall/edge/rules":
			_ = json.Unmarshal(body, &created)
			created.ID, created.Position = "fwr_1", 1
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(apiclient.FirewallEdgeRule{Rule: created})
		case r.URL.Path == "/api/v1/firewall/edge/rules":
			_ = json.NewEncoder(w).Encode([]apiclient.FirewallEdgeRule{{Rule: edgefw.Rule{ID: "fwr_1", Name: "n", Action: "deny", Enabled: true, Position: 1,
				Conditions: []edgefw.Condition{{Field: "path", Op: "starts_with", Value: "/x"}}}}})
		case r.URL.Path == "/api/v1/firewall/edge/block":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"fwr_2","position":1,"conditions":[]}`))
		case r.URL.Path == "/api/v1/firewall/edge/bans":
			_, _ = w.Write([]byte(`[{"ip":"198.51.100.4","reason":"rule","started":"s","ends":"e","hits":3}]`))
		case r.URL.Path == "/api/v1/firewall/edge/attack-mode":
			_, _ = w.Write([]byte(`{"active":true,"until":"2026-10-17T13:00:00Z","challenge":true}`))
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer srv.Close()
	run1 := func(args ...string) string {
		t.Helper()
		var stdout, stderr bytes.Buffer
		if got := run("levelrail-cli-test", append(args, "--api-url", srv.URL), &stdout, &stderr, envMap()); got != exitOK {
			t.Fatalf("%v exit = %d (stderr=%q)", args, got, stderr.String())
		}
		return stdout.String()
	}
	run1("firewall", "ingress", "rules", "add", "--name", "Block admin", "--action", "deny", "--when", "path starts_with /admin", "--when", "method equals POST")
	if created.Action != edgefw.ActionDeny || len(created.Conditions) != 2 || !created.Enabled || created.Conditions[1].Value != "POST" {
		t.Errorf("created = %+v", created)
	}
	if out := run1("firewall", "ingress", "rules", "list"); !strings.Contains(out, "path starts_with /x") {
		t.Errorf("list = %q", out)
	}
	run1("firewall", "ingress", "block", "203.0.113.0/24", "--note", "scanner", "--force")
	if out := run1("firewall", "ingress", "bans", "list"); !strings.Contains(out, "198.51.100.4") {
		t.Errorf("bans = %q", out)
	}
	run1("firewall", "ingress", "bans", "lift", "198.51.100.4")
	if out := run1("firewall", "ingress", "attack-mode", "on", "--minutes", "30", "--challenge"); !strings.Contains(out, "attack mode on") {
		t.Errorf("attack = %q", out)
	}
	joined := strings.Join(calls, ",")
	for _, want := range []string{"POST /api/v1/firewall/edge/block?force=true", "DELETE /api/v1/firewall/edge/bans/198.51.100.4", "POST /api/v1/firewall/edge/attack-mode"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing call %q in %s", want, joined)
		}
	}
}

func TestRun_FirewallIngress_UsageErrors(t *testing.T) {
	for _, args := range [][]string{
		{"firewall", "ingress"}, {"firewall", "ingress", "nope"}, {"firewall", "ingress", "rules"},
		{"firewall", "ingress", "attack-mode"}, {"firewall", "ingress", "bans", "x"},
	} {
		var stdout, stderr bytes.Buffer
		if got := run("levelrail-cli-test", args, &stdout, &stderr, envMap()); got != exitUsage {
			t.Errorf("%v exit = %d, want usage", args, got)
		}
	}
}
