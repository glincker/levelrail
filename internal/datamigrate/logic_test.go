package datamigrate

import (
	"strings"
	"testing"
)

func TestSourceValidate(t *testing.T) {
	tests := []struct {
		name    string
		engine  string
		src     Source
		wantErr string
		port    int
	}{
		{"postgres ok fills port", "postgres", Source{Host: "db.example.com", User: "u", Database: "app"}, "", 5432},
		{"mongo needs no database", "mongodb", Source{Host: "10.0.0.5", User: "u"}, "", 27017},
		{"redis ok", "redis", Source{Host: "cache"}, "", 6379},
		{"unsupported engine", "clickhouse", Source{Host: "h", Database: "d"}, "not supported", 0},
		{"empty host", "postgres", Source{Database: "d"}, "host is required", 0},
		{"metadata address", "postgres", Source{Host: "169.254.169.254", Database: "d"}, "link-local", 0},
		{"shell metacharacters in host", "postgres", Source{Host: "a;rm -rf", Database: "d"}, "hostname", 0},
		{"bad user", "mysql", Source{Host: "h", User: "x y", Database: "d"}, "user contains", 0},
		{"sql needs database", "mysql", Source{Host: "h", User: "u"}, "database is required", 0},
		{"port out of range", "postgres", Source{Host: "h", Port: 70000, Database: "d"}, "port must", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.src.Validate(tt.engine)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if tt.src.Port != tt.port {
					t.Errorf("port = %d, want %d", tt.src.Port, tt.port)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestCommandsKeepPasswordOutOfArguments(t *testing.T) {
	const pw = "s3cr3t-pw"
	src := Source{Host: "h", Port: 1, User: "u", Password: pw, Database: "d"}
	if got := helperEnv(src)[envPassword]; got != pw {
		t.Fatalf("helper env password = %q", got)
	}
	for _, engine := range []string{EnginePostgres, EngineMySQL, EngineMariaDB, EngineMongoDB, EngineRedis} {
		for name, fn := range map[string]func(string) ([]string, error){"dump": DumpCommand, "source count": SourceCountCommand, "target count": TargetCountCommand} {
			cmd, err := fn(engine)
			if err != nil {
				t.Fatalf("%s %s: %v", engine, name, err)
			}
			if strings.Contains(strings.Join(cmd, " "), pw) {
				t.Errorf("%s %s command contains the password", engine, name)
			}
		}
	}
	if _, err := DumpCommand("clickhouse"); err == nil {
		t.Error("an unsupported engine must be rejected")
	}
}

func TestParseCountsAndCompare(t *testing.T) {
	tests := []struct {
		name       string
		source     string
		target     string
		wantChecks int
		wantBad    int
	}{
		{"equal", "public.users|10\npublic.orders|0\n", "public.orders|0\npublic.users|10\n", 2, 0},
		{"count differs", "public.users|10\n", "public.users|9\n", 1, 1},
		{"missing on target", "a|1\nb|2\n", "a|1\n", 2, 1},
		{"extra on target ignored", "a|1\n", "a|1\nextra|5\n", 1, 0},
		{"notices ignored", "NOTICE: hi\na|3\n", "a|3\n\nSET\n", 1, 0},
		{"empty source", "", "a|1\n", 0, 0},
		{"pipe in name", "we|ird|4\n", "we|ird|4\n", 1, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := Compare(ParseCounts(tt.source), ParseCounts(tt.target))
			if v.Checked != tt.wantChecks || v.Mismatched != tt.wantBad {
				t.Fatalf("checked=%d mismatched=%d, want %d and %d", v.Checked, v.Mismatched, tt.wantChecks, tt.wantBad)
			}
		})
	}
	v := Compare(map[string]int64{"a": 1, "b": 2}, map[string]int64{"a": 1})
	if got := v.Failure(5); !strings.Contains(got, "b source=2 target=0") {
		t.Errorf("failure text = %q", got)
	}
}

func TestEvaluate(t *testing.T) {
	target := []string{"203.0.113.10"}
	tests := []struct {
		name        string
		in          EvalInput
		wantVerdict string
		wantChange  string
		wantCheck   string
	}{
		{
			name:        "ready, low ttl, still on source",
			in:          EvalInput{Ready: true, DNS: DomainState{Addrs: []string{"198.51.100.7"}, TTL: 60, Authoritative: true}, TargetIPs: target},
			wantVerdict: VerdictGo, wantChange: "A app.example.com 203.0.113.10",
		},
		{
			name:        "high ttl waits and says the exact record to lower",
			in:          EvalInput{Ready: true, DNS: DomainState{Addrs: []string{"198.51.100.7"}, TTL: 3600, Authoritative: true}, TargetIPs: target},
			wantVerdict: VerdictWait, wantChange: "A app.example.com 203.0.113.10", wantCheck: "set the TTL of app.example.com to 300",
		},
		{
			name:        "not ready is no-go even with low ttl",
			in:          EvalInput{Ready: false, ReadyDetail: "Attention needed", DNS: DomainState{Addrs: []string{"198.51.100.7"}, TTL: 60}, TargetIPs: target},
			wantVerdict: VerdictNoGo,
		},
		{
			name:        "already pointing here",
			in:          EvalInput{Ready: true, DNS: DomainState{Addrs: target, TTL: 86400}, TargetIPs: target},
			wantVerdict: VerdictSwitch,
		},
		{
			name:        "cname must be replaced",
			in:          EvalInput{Ready: true, DNS: DomainState{Addrs: []string{"198.51.100.7"}, CNAME: "old.example.net", TTL: 300, Authoritative: true}, TargetIPs: target},
			wantVerdict: VerdictGo, wantChange: "A app.example.com 203.0.113.10",
		},
		{
			name:        "ipv6 target uses AAAA",
			in:          EvalInput{Ready: true, DNS: DomainState{Addrs: []string{"198.51.100.7"}, TTL: 60}, TargetIPs: []string{"2001:db8::1"}},
			wantVerdict: VerdictGo, wantChange: "AAAA app.example.com 2001:db8::1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.in.App, tt.in.Domain = "web", "app.example.com"
			p := Evaluate(tt.in)
			if p.Verdict != tt.wantVerdict {
				t.Fatalf("verdict = %s, want %s (%+v)", p.Verdict, tt.wantVerdict, p.Checks)
			}
			switch {
			case tt.wantChange == "" && p.Change != nil && p.Verdict == VerdictSwitch:
				t.Errorf("a switched domain must not propose a change: %+v", p.Change)
			case tt.wantChange != "":
				if p.Change == nil || p.Change.Type+" "+p.Change.Name+" "+p.Change.Value != tt.wantChange {
					t.Errorf("change = %+v, want %s", p.Change, tt.wantChange)
				}
			}
			if tt.wantCheck != "" {
				var fixes []string
				for _, c := range p.Checks {
					fixes = append(fixes, c.Fix)
				}
				if !strings.Contains(strings.Join(fixes, "|"), tt.wantCheck) {
					t.Errorf("fixes = %v, want one containing %q", fixes, tt.wantCheck)
				}
			}
		})
	}
	if got := Overall([]DomainPlan{{Verdict: VerdictGo}, {Verdict: VerdictWait}, {Verdict: VerdictSwitch}}); got != VerdictWait {
		t.Errorf("overall = %s, want wait", got)
	}
	if got := Overall(nil); got != VerdictNoGo {
		t.Errorf("overall of nothing = %s, want no-go", got)
	}
}

func TestGuideVolumes(t *testing.T) {
	guides := GuideVolumes("web", "app-web-", "root@old.example.com", []VolumeRef{
		{Name: "app-web-uploads", ContainerPath: "/data"},
		{HostPath: "/srv/web/media", ContainerPath: "/media"},
	})
	if len(guides) != 2 {
		t.Fatalf("guides = %d", len(guides))
	}
	if g := guides[0]; g.Kind != "volume" || g.Source != "uploads" || !strings.Contains(g.Command, "root@old.example.com:/var/lib/docker/volumes/uploads/_data/") {
		t.Errorf("volume guide = %+v", g)
	}
	if g := guides[1]; g.Kind != "bind" || !strings.Contains(g.Command, "'root@old.example.com:/srv/web/media/' '/srv/web/media/'") {
		t.Errorf("bind guide = %+v", g)
	}
	for _, bad := range []string{"", "host", "a b@c", "u@h;x"} {
		if ValidateSSHTarget(bad) == nil {
			t.Errorf("ValidateSSHTarget(%q) accepted", bad)
		}
	}
}

func TestSourceScrub(t *testing.T) {
	src := Source{Password: "p@ss w/rd"}
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"raw", "auth failed for p@ss w/rd", "auth failed for [redacted]"},
		{"query escaped", "dial postgres://u:p%40ss+w%2Frd@h/db", "dial postgres://u:[redacted]@h/db"},
		{"path escaped", "uri p@ss%20w%2Frd end", "uri [redacted] end"},
		{"absent", "connection refused", "connection refused"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := src.Scrub(tc.in); got != tc.want {
				t.Fatalf("Scrub(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
	if got := (Source{}).Scrub("keep p@ss"); got != "keep p@ss" {
		t.Fatalf("empty password must not alter text, got %q", got)
	}
}
