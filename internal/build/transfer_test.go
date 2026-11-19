package build

import (
	"context"
	"errors"
	"io"
	"log/slog"
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
	have map[string]string
	err  error
}

func (f fakeVerifier) ManifestDigest(_ context.Context, repo, ref string) (string, error) {
	return f.have[repo+":"+ref], f.err
}

func TestVerifyPushedDigest(t *testing.T) {
	d := "sha256:" + strings.Repeat("a", 64)
	ref := "reg.example/levelrail/app:b1"
	v := fakeVerifier{have: map[string]string{"reg.example/levelrail/app:b1": d}}
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

func TestPushAuthConfigScopes(t *testing.T) {
	f := pushAuthConfig("reg.example", "levelrail/app", "tok")
	if got := f("reg.example", []string{"repository:levelrail/app:push,pull"}); got.Password != "tok" {
		t.Fatalf("job scope must get the token")
	}
	for name, c := range map[string]struct {
		host   string
		scopes []string
	}{
		"cache pull":   {"reg.example", []string{"repository:levelrail/app-cache:pull"}},
		"other repo":   {"reg.example", []string{"repository:library/alpine:pull"}},
		"pull only":    {"reg.example", []string{"repository:levelrail/app:pull"}},
		"extra scope":  {"reg.example", []string{"repository:levelrail/app:push,pull", "repository:x/y:pull"}},
		"other host":   {"other.example", []string{"repository:levelrail/app:push,pull"}},
		"nil other":    {"other.example", nil},
		"extra action": {"reg.example", []string{"repository:levelrail/app:push,pull,delete"}},
		"push only":    {"reg.example", []string{"repository:levelrail/app:push"}},
	} {
		if got := f(c.host, c.scopes); got.Password != "" || got.Username != "" {
			t.Errorf("%s: credential leaked", name)
		}
	}
}

func TestPushAuthConfigActionOrderAndNilScopes(t *testing.T) {
	f := pushAuthConfig("reg.example", "levelrail/app", "tok")
	for _, sc := range [][]string{
		{"repository:levelrail/app:pull,push"},
		{"repository:levelrail/app:push,pull"},
		{"levelrail/app:pull,push"},
		nil,
	} {
		if got := f("reg.example", sc); got.Password != "tok" {
			t.Errorf("scopes %v must get the token", sc)
		}
	}
}

type capNodes struct {
	capable map[string]bool
	pushed  []bool
}

func (c *capNodes) NodeSupportsBuildPush(id string) bool { return c.capable[id] }
func (c *capNodes) BuildOnNode(_ context.Context, _ string, req RemoteRequest, _ io.Writer, _ func(ProgressEvent)) (*Result, error) {
	c.pushed = append(c.pushed, req.Push != nil)
	return &Result{Digest: "sha256:" + strings.Repeat("a", 64)}, nil
}

type okVerifier struct{}

func (okVerifier) ManifestDigest(context.Context, string, string) (string, error) {
	return "sha256:" + strings.Repeat("a", 64), nil
}

func TestDispatchFollowsNodeCapability(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	nodes := &capNodes{capable: map[string]bool{"new": true}}
	r := &Router{remote: nodes, logger: slog.New(slog.DiscardHandler),
		load: func(_ context.Context, rd io.Reader, _ string, _ func(ProgressEvent)) error {
			_, err := io.Copy(io.Discard, rd)
			return err
		},
		transfer: func(context.Context) TransferConfig {
			return TransferConfig{Mode: TransferRegistry, TokenAuth: true, RegistryHost: "reg.example", TokenTTL: time.Minute,
				Issuer: testIssuer(t, &now), Verifier: okVerifier{},
				Pull: func(context.Context, string, string, string) error { return nil }}
		}}
	for _, id := range []string{"new", "old"} {
		if _, err := r.dispatchWith(context.Background(), id, RemoteRequest{Tag: "levelrail/app:b1"}, nil); err != nil {
			t.Fatalf("%s: %v", id, err)
		}
	}
	if len(nodes.pushed) != 2 || !nodes.pushed[0] || nodes.pushed[1] {
		t.Fatalf("capable node must use registry and old node tar, got %v", nodes.pushed)
	}
}

type failNodes struct {
	capNodes
	err   error
	calls []bool
}

func (f *failNodes) BuildOnNode(_ context.Context, _ string, req RemoteRequest, _ io.Writer, _ func(ProgressEvent)) (*Result, error) {
	f.calls = append(f.calls, req.Push != nil)
	if req.Push != nil {
		return nil, f.err
	}
	return &Result{Digest: "sha256:" + strings.Repeat("a", 64)}, nil
}

func TestDispatchFallsBackToTarOnPushAuthFailure(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	cfg := func(tokenAuth bool) func(context.Context) TransferConfig {
		return func(context.Context) TransferConfig {
			return TransferConfig{Mode: TransferRegistry, TokenAuth: tokenAuth, RegistryHost: "reg.example", TokenTTL: time.Minute,
				Issuer: testIssuer(t, &now), Verifier: okVerifier{},
				Pull: func(context.Context, string, string, string) error { return nil }}
		}
	}
	newRouter := func(n *failNodes, tokenAuth bool) *Router {
		return &Router{remote: n, logger: slog.New(slog.DiscardHandler), transfer: cfg(tokenAuth),
			load: func(_ context.Context, rd io.Reader, _ string, _ func(ProgressEvent)) error {
				_, err := io.Copy(io.Discard, rd)
				return err
			}}
	}
	n := &failNodes{capNodes: capNodes{capable: map[string]bool{"n": true}}, err: errors.New("push: failed to fetch anonymous token: 401 Unauthorized")}
	if _, err := newRouter(n, true).dispatchWith(context.Background(), "n", RemoteRequest{Tag: "levelrail/app:b1"}, nil); err != nil {
		t.Fatalf("auth failure must fall back: %v", err)
	}
	if len(n.calls) != 2 || !n.calls[0] || n.calls[1] {
		t.Fatalf("want registry then tar, got %v", n.calls)
	}
	n = &failNodes{capNodes: capNodes{capable: map[string]bool{"n": true}}, err: errors.New("compile error in step 3")}
	if _, err := newRouter(n, true).dispatchWith(context.Background(), "n", RemoteRequest{Tag: "levelrail/app:b1"}, nil); err == nil || len(n.calls) != 1 {
		t.Fatalf("a real build failure must not fall back: %v %v", err, n.calls)
	}
	n = &failNodes{capNodes: capNodes{capable: map[string]bool{"n": true}}}
	if _, err := newRouter(n, false).dispatchWith(context.Background(), "n", RemoteRequest{Tag: "levelrail/app:b1"}, nil); err != nil || len(n.calls) != 1 || n.calls[0] {
		t.Fatalf("registry not in token mode must use tar: %v %v", err, n.calls)
	}
}

func TestPushAuthFailureClassifiesRealmRefusal(t *testing.T) {
	// The wording buildkit reports when the token realm answers a bare 401 after job end.
	err := errors.New(`push: failed to fetch oauth token: unexpected status from GET request to https://cp/api/v1/registry/token: 401 Unauthorized`)
	if !isPushAuthFailure(err) {
		t.Fatal("a refused token realm must select the tar fallback")
	}
}
