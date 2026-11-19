package build

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func testIssuer(t *testing.T, now *time.Time) *PushTokenIssuer {
	t.Helper()
	i, err := NewPushTokenIssuer([]byte(strings.Repeat("k", 32)))
	if err != nil {
		t.Fatal(err)
	}
	i.now = func() time.Time { return *now }
	return i
}

func TestPushTokenScopeAndExpiry(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	i := testIssuer(t, &now)
	tok, err := i.Issue("reg.example/levelrail/app", "b1", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := i.Verify(tok, "reg.example/levelrail/app", "b1", "push"); err != nil {
		t.Fatalf("valid token rejected: %v", err)
	}
	for name, args := range map[string][3]string{
		"other repo": {"reg.example/levelrail/other", "b1", "push"},
		"other tag":  {"reg.example/levelrail/app", "b2", "push"},
		"pull":       {"reg.example/levelrail/app", "b1", "pull"},
	} {
		if err := i.Verify(tok, args[0], args[1], args[2]); !errors.Is(err, ErrPushTokenScope) {
			t.Errorf("%s: want scope error, got %v", name, err)
		}
	}
	if err := i.Verify(tok+"x", "reg.example/levelrail/app", "b1", "push"); !errors.Is(err, ErrPushTokenInvalid) {
		t.Errorf("tampered token: got %v", err)
	}
	now = now.Add(2 * time.Minute)
	if err := i.Verify(tok, "reg.example/levelrail/app", "b1", "push"); !errors.Is(err, ErrPushTokenExpired) {
		t.Errorf("expired token: got %v", err)
	}
	g := PushGrant{Ref: "r/a:b", Token: tok}
	if strings.Contains(g.String(), tok) || strings.Contains(g.LogValue().String(), tok) {
		t.Error("token leaked through String or LogValue")
	}
}

type fakeVerifier struct {
	have map[string]bool
	err  error
}

func (f fakeVerifier) ManifestExists(_ context.Context, repo, digest string) (bool, error) {
	return f.have[repo+"@"+digest], f.err
}

func TestVerifyPushedDigest(t *testing.T) {
	d := "sha256:" + strings.Repeat("a", 64)
	ref := "reg.example/levelrail/app:b1"
	v := fakeVerifier{have: map[string]bool{"reg.example/levelrail/app@" + d: true}}
	if err := VerifyPushedDigest(context.Background(), v, ref, d); err != nil {
		t.Fatalf("existing digest rejected: %v", err)
	}
	if err := VerifyPushedDigest(context.Background(), v, ref, "sha256:"+strings.Repeat("b", 64)); !errors.Is(err, ErrDigestMissing) {
		t.Errorf("missing digest: got %v", err)
	}
	if err := VerifyPushedDigest(context.Background(), v, ref, "latest"); err == nil {
		t.Error("malformed digest accepted")
	}
	if err := VerifyPushedDigest(context.Background(), fakeVerifier{err: errors.New("down")}, ref, d); err == nil {
		t.Error("registry error must fail closed")
	}
}
