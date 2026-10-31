package egress

import (
	"fmt"
	"net/url"
	"strings"
	"testing"
)

func credProxy(t *testing.T, d *denials) (string, string) {
	t.Helper()
	port := echoServer(t)
	host := fmt.Sprintf("api.test:%d", port)
	allow := StaticAllowlist{"mailer": {host}, "other": {"elsewhere.test"}}
	s := newProxy(t, true, allow, fakeResolver{"api.test": {"127.0.0.1"}}, Config{AllowPrivate: true}, d)
	return s.URL, host
}

func TestCredentialRequired(t *testing.T) {
	d := &denials{}
	u, host := credProxy(t, d)
	tests := []struct {
		name, user, pass, status string
	}{
		{"missing credentials", "", "", "407"},
		{"wrong secret", "mailer", "nope", "407"},
		{"unknown plugin", "ghost", testSecret, "407"},
		{"right secret", "mailer", testSecret, "200"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			st, _, _ := connectAs(t, u, host, tc.user, tc.pass)
			if !strings.Contains(st, " "+tc.status+" ") {
				t.Fatalf("status = %q, want %s", st, tc.status)
			}
		})
	}
}

func TestChallengeHeaderAndNoDetail(t *testing.T) {
	u, host := credProxy(t, &denials{})
	st1, c1, br1 := connectAs(t, u, host, "mailer", "nope")
	st2, _, _ := connectAs(t, u, host, "ghost", "nope")
	if st1 != st2 {
		t.Fatalf("responses differ: %q vs %q", st1, st2)
	}
	_ = c1
	var head strings.Builder
	for {
		l, err := br1.ReadString('\n')
		if err != nil || strings.TrimSpace(l) == "" {
			break
		}
		head.WriteString(l)
	}
	if !strings.Contains(head.String(), "Proxy-Authenticate: Basic") {
		t.Fatalf("missing challenge in %q", head.String())
	}
}

func TestUsernameSwapDoesNotBorrowAllowlist(t *testing.T) {
	u, host := credProxy(t, &denials{})
	// "other" exists but the secret belongs to "mailer", whose list covers host.
	st, _, _ := connectAs(t, u, host, "other", testSecret)
	if !strings.Contains(st, " 407 ") {
		t.Fatalf("status = %q", st)
	}
	// With its own secret "other" is authenticated but its list excludes host.
	st, _, _ = connectAs(t, u, host, "other", "other-secret")
	if !strings.Contains(st, " 403 ") {
		t.Fatalf("status = %q", st)
	}
}

func TestSecretAbsentFromDenials(t *testing.T) {
	d := &denials{}
	u, host := credProxy(t, d)
	connectAs(t, u, host, "mailer", testSecret+"x")
	connectAs(t, u, host, "other", "other-secret")
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.list) != 2 {
		t.Fatalf("denials = %v", d.list)
	}
	for _, e := range d.list {
		if strings.Contains(e, testSecret) || strings.Contains(e, "other-secret") || strings.Contains(e, "mailer") {
			t.Fatalf("denial leaks credential data: %q", e)
		}
	}
}

func TestNewCredentialAndVerify(t *testing.T) {
	s1, h1, err := NewCredential()
	if err != nil {
		t.Fatal(err)
	}
	s2, _, _ := NewCredential()
	if s1 == s2 || len(s1) < 43 || h1 != HashSecret(s1) {
		t.Fatal("credential must be unique, long and hash to its secret")
	}
	st := NewHashedCredentials(map[string]SecretHash{"p": h1})
	if !st.Verify("p", s1) || st.Verify("p", s2) || st.Verify("q", s1) {
		t.Fatal("verify mismatch")
	}
}

func TestProxyURLEscaping(t *testing.T) {
	odd := "a@b:c/d?e#f%g h"
	raw := ProxyURL("mailer", odd, "proxy.internal:3128")
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	pw, _ := u.User.Password()
	if u.User.Username() != "mailer" || pw != odd || u.Host != "proxy.internal:3128" {
		t.Fatalf("round trip failed: %q", raw)
	}
	if strings.Count(raw, "@") != 1 {
		t.Fatalf("userinfo not escaped: %q", raw)
	}
}
