package api

import (
	"context"
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/certflow"
	"github.com/GLINCKER/levelrail/internal/domaindoctor"
)

type reachFake struct {
	err error
	dir string
}

func (f *reachFake) ProveHTTP01(_ context.Context, dir, _ string) error {
	f.dir = dir
	return f.err
}

func reachRouter(o certflow.Orderer) *Router {
	rt := &Router{httpsMaxAttemptsPerHour: 5}
	WithReachabilityOrderer(o)(rt)
	return rt
}

func port80(rep domaindoctor.Report) (domaindoctor.Check, bool) {
	for _, c := range rep.Checks {
		if c.ID == domaindoctor.IDPort80 {
			return c, true
		}
	}
	return domaindoctor.Check{}, false
}

func TestGoLiveReachProofOpenAndClosed(t *testing.T) {
	selfFail := domaindoctor.Report{Probed: true, Checks: []domaindoctor.Check{{ID: domaindoctor.IDPort80, State: domaindoctor.StateFail}}}
	f := &reachFake{}
	rt := reachRouter(f)
	if c, _ := port80(rt.withOutsideProof(context.Background(), selfFail, "a.example.dev")); c.State != domaindoctor.StatePass || f.dir != ACMEStagingDirectoryURL {
		t.Fatalf("open: %+v dir %q", c, f.dir)
	}
	fc := &reachFake{err: &certflow.ValidationError{Detail: "connection refused"}}
	c, ok := port80(reachRouter(fc).withOutsideProof(context.Background(), domaindoctor.Report{Probed: true}, "b.example.dev"))
	if !ok || c.State != domaindoctor.StateFail {
		t.Fatalf("closed: %+v", c)
	}
}

func TestGoLiveReachProofKeepsSelfCheckWhenUnknown(t *testing.T) {
	rep := domaindoctor.Report{Probed: true, Checks: []domaindoctor.Check{{ID: domaindoctor.IDPort80, State: domaindoctor.StatePass}}}
	rt := reachRouter(&reachFake{err: errors.New("ca down")})
	if c, _ := port80(rt.withOutsideProof(context.Background(), rep, "a.example.dev")); c.State != domaindoctor.StatePass || c.Title != "" {
		t.Fatalf("self-check replaced: %+v", c)
	}
	if got := (&Router{}).withOutsideProof(context.Background(), rep, "a.example.dev"); len(got.Checks) != 1 {
		t.Fatal("no orderer must leave the report alone")
	}
}
