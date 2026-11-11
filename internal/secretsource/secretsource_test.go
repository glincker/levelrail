package secretsource

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestParseRef(t *testing.T) {
	good := map[string]Ref{
		"secret://prod-vault/apps/web#db_url":       {Conn: "prod-vault", Path: "apps/web", Key: "db_url"},
		"secret://v1/a#K?version=3":                 {Conn: "v1", Path: "a", Key: "K", Version: 3},
		"secret://doppler/proj.cfg/Dev_Env#API-KEY": {Conn: "doppler", Path: "proj.cfg/Dev_Env", Key: "API-KEY"},
		"secret://a/..foo/b#k":                      {Conn: "a", Path: "..foo/b", Key: "k"},
	}
	for in, want := range good {
		got, err := ParseRef(in)
		if err != nil || got != want {
			t.Errorf("%q: got %+v err %v", in, got, err)
		}
		if err == nil {
			if again, err := ParseRef(got.String()); err != nil || again != got {
				t.Errorf("%q: round trip failed", in)
			}
		}
	}
	bad := []string{
		"", "secret://", "secret://c", "secret://c/p", "secret://c/p#", "secret://c#k", "secret:///p#k",
		"vault://c/p#k", "SECRET://c/p#k", "secret://Prod/p#k", "secret://-c/p#k", "secret://c d/p#k",
		"secret://c/../etc#k", "secret://c/a/../b#k", "secret://c/./a#k", "secret://c//a#k", "secret://c/a/#k",
		"secret://c/%2e%2e/a#k", "secret://c/a%2fb#k", "secret://c/a\\b#k", "secret://c/a b#k",
		"secret://c/a\x00b#k", "secret://c/a\nb#k", "secret://c/p#k#j", "secret://c/p?x=1#k",
		"secret://c/p#k?version=0", "secret://c/p#k?version=-1", "secret://c/p#k?version=01",
		"secret://c/p#k?version=", "secret://c/p#k?v=1", "secret://c/p#k?version=1&x=2",
		"secret://c/p#..", "secret://c/p#k/x", "secret://c/pé#k", "secret://c/p#k\u202e",
		"secret://c/" + strings.Repeat("a", 600) + "#k", "secret://" + strings.Repeat("c", 64) + "/p#k",
		"secret://c/p#" + strings.Repeat("k", 129), "secret://c/" + strings.Repeat("a/", 17) + "a#k",
		"secret://c/" + strings.Repeat("a", 2000) + "#k",
	}
	for _, in := range bad {
		if _, err := ParseRef(in); !errors.Is(err, ErrInvalidRef) {
			t.Errorf("%q: want ErrInvalidRef, got %v", in, err)
		}
	}
}

func TestParseRefLimitsFromEnv(t *testing.T) {
	t.Setenv("LEVELRAIL_SECRETREF_MAX_PATH", "4")
	if _, err := ParseRef("secret://c/abcde#k"); err == nil {
		t.Fatal("env path cap not applied")
	}
	if _, err := ParseRef("secret://c/abcd#k"); err != nil {
		t.Fatal(err)
	}
}

func TestErrorDoesNotEchoInput(t *testing.T) {
	_, err := ParseRef("secret://c/p#k?version=hunter2pass")
	if err == nil || strings.Contains(err.Error(), "hunter2pass") {
		t.Fatalf("error echoes input: %v", err)
	}
}

func TestObjectForm(t *testing.T) {
	r, err := Spec{Secret: RefSpec{Conn: "v", Path: "a/b", Key: "k", Version: 2}}.Ref()
	if err != nil || r.String() != "secret://v/a/b#k?version=2" {
		t.Fatalf("%v %v", r, err)
	}
	for _, s := range []RefSpec{{Conn: "V", Path: "a", Key: "k"}, {Conn: "v", Path: "../a", Key: "k"}, {Conn: "v", Path: "a", Key: ""}, {Conn: "v", Path: "a", Key: "k", Version: -1}} {
		if _, err := s.Ref(); !errors.Is(err, ErrInvalidRef) {
			t.Errorf("%+v accepted", s)
		}
	}
}

func newTestResolver(t *testing.T) (*Resolver, *FakeProvider, *FakeProvider) {
	t.Helper()
	a := NewFake("a").Set("apps/web", "pw", "s3cretvalue").SetVersion("apps/web", "pw", 2, "oldvalue")
	b := NewFake("b").Set("x", "y", "other")
	r := NewResolver()
	if err := r.Register("one", a); err != nil {
		t.Fatal(err)
	}
	if err := r.Register("two", b); err != nil {
		t.Fatal(err)
	}
	return r, a, b
}

func TestResolverDispatch(t *testing.T) {
	r, a, b := newTestResolver(t)
	ctx := context.Background()
	v, err := r.Resolve(ctx, Ref{Conn: "one", Path: "apps/web", Key: "pw"})
	if err != nil || string(v.Reveal()) != "s3cretvalue" {
		t.Fatalf("%v", err)
	}
	v, err = r.Resolve(ctx, Ref{Conn: "one", Path: "apps/web", Key: "pw", Version: 2})
	if err != nil || string(v.Reveal()) != "oldvalue" {
		t.Fatalf("version: %v", err)
	}
	if _, err = r.Resolve(ctx, Ref{Conn: "two", Path: "x", Key: "y"}); err != nil {
		t.Fatal(err)
	}
	if a.Calls() != 2 || b.Calls() != 1 {
		t.Fatalf("dispatch counts %d %d", a.Calls(), b.Calls())
	}
	if err := r.Register("one", a); err == nil {
		t.Fatal("rebind allowed")
	}
	if err := r.Register("Bad", a); !errors.Is(err, ErrInvalidRef) {
		t.Fatal("bad conn name allowed")
	}
}

func TestResolverFailClosed(t *testing.T) {
	r, a, _ := newTestResolver(t)
	ctx := context.Background()
	ref := Ref{Conn: "one", Path: "apps/web", Key: "pw"}

	if _, err := r.Resolve(ctx, Ref{Conn: "nope", Path: "p", Key: "k"}); !errors.Is(err, ErrUnknownConnection) || !errors.Is(err, ErrFailClosed) {
		t.Errorf("unknown: %v", err)
	}
	if _, err := r.Resolve(ctx, Ref{Conn: "one", Path: "apps/web", Key: "zz"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("not found: %v", err)
	}
	a.Deny("apps/web")
	if _, err := r.Resolve(ctx, ref); !errors.Is(err, ErrDenied) {
		t.Errorf("denied: %v", err)
	}
	a.denied = map[string]bool{}
	a.FailWith(errors.New("dial tcp: connection refused"))
	_, err := r.Resolve(ctx, ref)
	if !errors.Is(err, ErrUnavailable) || !errors.Is(err, ErrFailClosed) {
		t.Errorf("unavailable: %v", err)
	}
	a.FailWith(nil)
	a.Set("empty", "k", "")
	if _, err := r.Resolve(ctx, Ref{Conn: "one", Path: "empty", Key: "k"}); !errors.Is(err, ErrFailClosed) {
		t.Errorf("empty value: %v", err)
	}
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := r.Resolve(cctx, ref); !errors.Is(err, ErrUnavailable) {
		t.Errorf("cancelled: %v", err)
	}
	if _, err := r.Resolve(ctx, Ref{Conn: "one", Path: "../x", Key: "k"}); !errors.Is(err, ErrInvalidRef) {
		t.Errorf("traversal: %v", err)
	}
}

func TestRedaction(t *testing.T) {
	r, a, _ := newTestResolver(t)
	ctx := context.Background()
	ref := Ref{Conn: "one", Path: "apps/web", Key: "pw"}
	if _, err := r.Resolve(ctx, ref); err != nil {
		t.Fatal(err)
	}
	// A provider that leaks the value in its error must be masked.
	a.FailWith(fmt.Errorf("auth failed for s3cretvalue and oldvalue"))
	_, err := r.Resolve(ctx, ref)
	if err == nil || strings.Contains(err.Error(), "s3cretvalue") {
		t.Fatalf("leak: %v", err)
	}
	if !errors.Is(err, ErrUnavailable) {
		t.Errorf("kind lost: %v", err)
	}
	if got := r.Redactor().Mask("x s3cretvalue y"); got != "x [redacted] y" {
		t.Errorf("mask: %q", got)
	}
	v := NewValue([]byte("topsecret"), "1")
	for _, s := range []string{v.String(), fmt.Sprintf("%v %+v %#v %s", v, v, v, v)} {
		if strings.Contains(s, "topsecret") {
			t.Errorf("value leaked via fmt: %s", s)
		}
	}
}

func TestRedactorLongestFirst(t *testing.T) {
	rd := NewRedactor()
	rd.Add([]byte("abc"))
	rd.Add([]byte("abcdef"))
	rd.Add(nil)
	if got := rd.Mask("abcdef"); got != Mask {
		t.Errorf("got %q", got)
	}
	if rd.MaskError(nil) != nil {
		t.Error("nil error")
	}
}

type renewer struct{ *FakeProvider }

func (renewer) Renew(_ context.Context, l Lease) (Lease, error) { return l, nil }

func TestRenew(t *testing.T) {
	r, _, _ := newTestResolver(t)
	if _, err := r.Renew(context.Background(), "one", Lease{ID: "l"}); !errors.Is(err, ErrUnsupported) {
		t.Errorf("want unsupported: %v", err)
	}
	f := NewFake("r")
	f.caps.Renew = true
	if err := r.Register("dyn", renewer{f}); err != nil {
		t.Fatal(err)
	}
	if l, err := r.Renew(context.Background(), "dyn", Lease{ID: "l"}); err != nil || l.ID != "l" {
		t.Errorf("renew: %v", err)
	}
	if _, err := r.Renew(context.Background(), "zz", Lease{}); !errors.Is(err, ErrUnknownConnection) {
		t.Error("unknown conn")
	}
}
