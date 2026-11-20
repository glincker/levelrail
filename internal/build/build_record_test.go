package build

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
)

func recordRouter(tier string, rec RecordSigner) *Router {
	return &Router{
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		nodes:    func(context.Context) ([]NodeInfo, error) { return []NodeInfo{{ID: "n1", Trust: tier}}, nil },
		recorder: rec,
	}
}

func TestRecordBuildPersonalFailsClosed(t *testing.T) {
	res := &Result{Digest: "sha256:abc"}
	if err := recordRouter(TrustPersonal, nil).recordBuild(context.Background(), "n1", "h/app:t", "h/app:t", res); err == nil {
		t.Fatal("personal builder with no signer must fail")
	}
	failing := func(context.Context, RecordInput) (string, error) { return "", errors.New("down") }
	if err := recordRouter(TrustPersonal, failing).recordBuild(context.Background(), "n1", "h/app:t", "h/app:t", res); err == nil {
		t.Fatal("personal builder with a failing signer must fail")
	}
	if err := recordRouter(TrustManaged, failing).recordBuild(context.Background(), "n1", "h/app:t", "h/app:t", res); err != nil {
		t.Fatalf("managed builder tolerates a signer failure: %v", err)
	}
}

func TestRecordBuildStampsRecordID(t *testing.T) {
	var got RecordInput
	rec := func(_ context.Context, in RecordInput) (string, error) { got = in; return "rec1", nil }
	r := recordRouter(TrustPersonal, rec)
	res := &Result{Digest: "sha256:abc"}
	if err := r.recordBuild(context.Background(), "n1", "h/app:t", "h/app:t", res); err != nil {
		t.Fatal(err)
	}
	if res.RecordID != "rec1" || got.App != "app" || got.BuilderTrust != TrustPersonal || got.Digest != "sha256:abc" {
		t.Fatalf("unexpected record input %+v result %+v", got, res)
	}
}

func TestMarkBuilderTarPathHasNoRecord(t *testing.T) {
	res := &Result{}
	recordRouter(TrustServer, nil).markBuilder(context.Background(), res, "n1")
	if res.RecordID != "" || res.BuilderTrust != TrustServer || res.BuilderNodeID != "n1" {
		t.Fatalf("tar result changed: %+v", res)
	}
	recordRouter(TrustServer, nil).markBuilder(context.Background(), nil, "n1")
}
