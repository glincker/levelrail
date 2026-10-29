package backup

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestLocalTargetRoundTripAndConfinement(t *testing.T) {
	root := t.TempDir()
	r := Routed{LocalRoot: root}
	d := Destination{Endpoint: LocalEndpointScheme, Bucket: "dumps"}
	ctx := context.Background()

	if err := r.Test(ctx, d); err != nil {
		t.Fatalf("test: %v", err)
	}
	if err := r.Upload(ctx, d, "db/a.dump", strings.NewReader("data"), 4); err != nil {
		t.Fatalf("upload: %v", err)
	}
	rc, err := r.Download(ctx, d, "db/a.dump")
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	b, _ := io.ReadAll(rc)
	_ = rc.Close()
	if string(b) != "data" {
		t.Fatalf("got %q", b)
	}
	objs, err := r.List(ctx, d, "db/")
	if err != nil || len(objs) != 1 || objs[0].Key != "db/a.dump" {
		t.Fatalf("list: %v %v", objs, err)
	}
	if err := r.Delete(ctx, d, "db/a.dump"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := r.Upload(ctx, d, "../../escape", strings.NewReader("x"), 1); err == nil {
		t.Fatal("key traversal must be refused")
	}
	if err := r.Upload(ctx, Destination{Endpoint: LocalEndpointScheme, Bucket: "../out"}, "k", strings.NewReader("x"), 1); err == nil {
		t.Fatal("bucket traversal must be refused")
	}
	if err := (Routed{}).Upload(ctx, d, "k", strings.NewReader("x"), 1); !errors.Is(err, ErrLocalRootUnset) {
		t.Fatalf("want ErrLocalRootUnset, got %v", err)
	}
}
