package api

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"testing"
)

type digestVerifierStub struct {
	stubResolver
	exists bool
	err    error
}

func (d digestVerifierStub) ImageDigestExists(context.Context, string) (bool, error) {
	return d.exists, d.err
}

func TestVerifyPromotionDigest(t *testing.T) {
	const pinned = "web:1@sha256:abc"
	cases := []struct {
		name   string
		image  string
		stub   digestVerifierStub
		status int
	}{
		{"missing digest is gone", pinned, digestVerifierStub{}, http.StatusGone},
		{"present digest passes", pinned, digestVerifierStub{exists: true}, 0},
		{"inconclusive check passes", pinned, digestVerifierStub{err: errors.New("timeout")}, 0},
		{"bare tag skips the check", "web:1", digestVerifierStub{}, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rt := &Router{logger: slog.New(slog.NewTextHandler(io.Discard, nil)), imageResolver: c.stub}
			err := rt.verifyPromotionDigest(context.Background(), c.image)
			var de *deployApprovalDecisionError
			switch {
			case c.status == 0 && err != nil:
				t.Fatalf("err = %v, want nil", err)
			case c.status != 0 && (!errors.As(err, &de) || de.status != c.status):
				t.Fatalf("err = %v, want status %d", err, c.status)
			}
		})
	}
}
