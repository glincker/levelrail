package objectstore

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/objectstore/objectstoretest"
	"github.com/GLINCKER/levelrail/kit/netguard"
)

func TestScopedKey(t *testing.T) {
	tests := []struct {
		key  string
		want string
		ok   bool
	}{
		{"a/b.txt", "buckets/x/a/b.txt", true},
		{"/lead.txt", "buckets/x/lead.txt", true},
		{"", "", false},
		{"../other/key", "", false},
		{"a/../../b", "", false},
	}
	for _, tt := range tests {
		got, err := ScopedKey("buckets/x/", tt.key)
		if (err == nil) != tt.ok || got != tt.want {
			t.Errorf("ScopedKey(%q) = %q, %v; want %q ok=%v", tt.key, got, err, tt.want, tt.ok)
		}
	}
}

func TestSweepExpiredAndUsage(t *testing.T) {
	t.Setenv(netguard.AllowPrivateEnv, "true")
	srv := objectstoretest.New("b")
	t.Cleanup(srv.Close)
	now := time.Now()
	srv.Put("buckets/x/old.txt", []byte("12345"), now.AddDate(0, 0, -10))
	srv.Put("buckets/x/new.txt", []byte("123"), now)
	srv.Put("buckets/y/other.txt", []byte("1"), now.AddDate(0, 0, -10))

	c, err := New(Config{Endpoint: srv.URL, Region: "auto", Bucket: "b", AccessKeyID: "k", SecretAccessKey: "s", PathStyle: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	bytes, objects, err := c.Usage(ctx, "buckets/x/")
	if err != nil || bytes != 8 || objects != 2 {
		t.Fatalf("Usage = %d, %d, %v; want 8, 2", bytes, objects, err)
	}
	res, err := c.SweepExpired(ctx, "buckets/x/", now.AddDate(0, 0, -7))
	if err != nil || res.Deleted != 1 || res.Bytes != 5 {
		t.Fatalf("SweepExpired = %+v, %v; want 1 deleted, 5 bytes", res, err)
	}
	if _, ok := srv.Data("buckets/y/other.txt"); !ok {
		t.Fatal("sweep crossed into another bucket prefix")
	}
	url, err := c.PresignGet(ctx, "buckets/x/new.txt", time.Minute)
	if err != nil || url == "" {
		t.Fatalf("PresignGet = %q, %v", url, err)
	}
}
