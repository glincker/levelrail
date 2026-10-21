package mailgroups

import (
	"testing"
	"time"
)

func TestResolveRules(t *testing.T) {
	now := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	old := now.AddDate(0, 0, -120)
	recent := now.AddDate(0, 0, -2)
	dir := Directory{
		ProjectTeams: map[string]string{"p1": "t2"},
		Users: []User{
			{ID: "u1", Email: "Ann@corp.example", Role: "admin", TeamIDs: []string{"t1"}, LastLoginAt: &recent, CreatedAt: old},
			{ID: "u2", Email: "bob@corp.example", Role: "viewer", TeamIDs: []string{"t2"}, LastLoginAt: &old, CreatedAt: old},
			{ID: "u3", Email: "cy@other.example", Role: "viewer", CreatedAt: old},
		},
	}
	cases := []struct {
		name  string
		rules Rules
		want  []string
	}{
		{"all", Rules{AllUsers: true}, []string{"ann@corp.example", "bob@corp.example", "cy@other.example"}},
		{"role", Rules{Roles: []string{"admin"}}, []string{"ann@corp.example"}},
		{"domain", Rules{EmailDomains: []string{"corp.example"}}, []string{"ann@corp.example", "bob@corp.example"}},
		{"project owner team", Rules{Projects: []string{"p1"}}, []string{"bob@corp.example"}},
		{"inactive only", Rules{InactiveDays: 30}, []string{"bob@corp.example", "cy@other.example"}},
		{"role and inactive", Rules{Roles: []string{"viewer"}, InactiveDays: 30}, []string{"bob@corp.example", "cy@other.example"}},
		{"extra and exclude", Rules{Roles: []string{"admin"}, ExtraAddresses: []string{"x@y.example", "ann@corp.example"}, ExcludeAddresses: []string{"ann@corp.example"}}, []string{"x@y.example"}},
	}
	for _, c := range cases {
		var got []string
		for _, r := range Resolve(dir, c.rules, now) {
			got = append(got, r.Email)
		}
		if len(got) != len(c.want) {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%s: got %v want %v", c.name, got, c.want)
			}
		}
	}
}

func TestSplitHonorsSuppressionOnlyWhenAsked(t *testing.T) {
	r := []Recipient{{Email: "a@x.example"}, {Email: "b@x.example"}}
	send, held := Split(r, map[string]bool{"b@x.example": true}, true)
	if len(send) != 1 || len(held) != 1 {
		t.Fatalf("send=%v held=%v", send, held)
	}
	send, held = Split(r, map[string]bool{"b@x.example": true}, false)
	if len(send) != 2 || len(held) != 0 {
		t.Fatalf("system mail must ignore opt-out: send=%v held=%v", send, held)
	}
}

func TestUnsubscribeTokenIsSigned(t *testing.T) {
	key := []byte("test-key")
	tok := SignUnsubscribe(key, "Ann@Corp.example")
	if got, ok := VerifyUnsubscribe(key, tok); !ok || got != "ann@corp.example" {
		t.Fatalf("verify = %q, %v", got, ok)
	}
	if _, ok := VerifyUnsubscribe([]byte("other"), tok); ok {
		t.Error("token verified under a different key")
	}
	forged := SignUnsubscribe(key, "victim@x.example")
	enc, _, _ := cutDot(forged)
	if _, ok := VerifyUnsubscribe(key, enc+".AAAA"); ok {
		t.Error("forged signature accepted")
	}
}

func cutDot(s string) (string, string, bool) {
	for i := 0; i < len(s); i++ {
		if s[i] == '.' {
			return s[:i], s[i+1:], true
		}
	}
	return s, "", false
}
