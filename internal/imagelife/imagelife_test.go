package imagelife

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
)

func rec(id string, size int64, age time.Duration, now time.Time, tags ...string) docker.ImageRecord {
	return docker.ImageRecord{ID: id, Tags: tags, SizeBytes: size, CreatedAt: now.Add(-age)}
}

func TestBuildAndPlanGC(t *testing.T) {
	now := time.Now()
	day := 24 * time.Hour
	records := []docker.ImageRecord{
		rec("sha256:live", 100, 10*day, now, "app:v5"),
		rec("sha256:pinned", 200, 20*day, now, "pin/app:abc"),
		rec("sha256:recent", 300, 5*day, now, "app:v4"),
		rec("sha256:old", 400, 30*day, now, "app:v1"),
		rec("sha256:young", 500, time.Hour, now, "app:v0"),
		rec("sha256:foreign", 600, 40*day, now, "postgres:16"),
		{ID: "sha256:busy", SizeBytes: 700, Containers: 1, CreatedAt: now.Add(-50 * day), Tags: []string{"app:v2"}},
	}
	app := AppState{
		Name:      "web",
		LiveImage: "app:v5",
		Pins:      []Pin{{DeployID: "d3", ImageID: "sha256:pinned", PinRef: "pin/app:abc"}},
		Attempts: []Attempt{
			{ID: "d5", Image: "app:v5", Status: "succeeded"},
			{ID: "d4", Image: "app:v4", Status: "succeeded"},
			{ID: "d0", Image: "app:v0", Status: "succeeded"},
			{ID: "d2", Image: "app:v2", Status: "succeeded"},
			{ID: "d1", Image: "app:v1", Status: "succeeded"},
		},
	}
	entries := Build(records, []AppState{app}, 2, 24*time.Hour, now)
	plan := PlanGC(entries)
	if len(plan.Remove) != 1 || plan.Remove[0].ID != "sha256:old" {
		t.Fatalf("remove = %+v, want only sha256:old", plan.Remove)
	}
	for _, e := range entries {
		if e.ID == "sha256:foreign" && !e.Protected {
			t.Error("unattributed image must be protected")
		}
		if e.ID == "sha256:pinned" && e.ProtectReason != "pinned rollback target" {
			t.Errorf("pinned reason = %q", e.ProtectReason)
		}
	}
}

func TestRemoveRefs(t *testing.T) {
	if got := RemoveRefs(Entry{ID: "x", Tags: []string{"a:1", "a:2"}}); len(got) != 2 {
		t.Errorf("tagged refs = %v", got)
	}
	if got := RemoveRefs(Entry{ID: "x"}); len(got) != 1 || got[0] != "x" {
		t.Errorf("untagged refs = %v", got)
	}
}

func TestVerify(t *testing.T) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	pub, err := ParsePublicKey(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
	if err != nil {
		t.Fatal(err)
	}
	digest := "sha256:" + strings.Repeat("ab", 32)
	payload, sig, err := SignPayload(priv, "localhost/app", digest)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(payload)
	blobDigest := "sha256:" + hex.EncodeToString(sum[:])
	sigTag := "sha256-" + strings.Repeat("ab", 32) + ".sig"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/app/manifests/" + sigTag:
			_ = json.NewEncoder(w).Encode(map[string]any{"layers": []map[string]any{{
				"digest": blobDigest, "annotations": map[string]string{cosignSigAnnotation: sig},
			}}})
		case "/v2/app/blobs/" + blobDigest:
			_, _ = w.Write(payload)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	host := strings.TrimPrefix(srv.URL, "http://")
	// httptest binds 127.0.0.1, which regClient treats as plain http.
	v := NewVerifier()
	ctx := context.Background()
	if err := v.Verify(ctx, host+"/app@"+digest, pub, Credentials{}); err != nil {
		t.Fatalf("valid signature rejected: %v", err)
	}
	other, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err := v.Verify(ctx, host+"/app@"+digest, &other.PublicKey, Credentials{}); err == nil {
		t.Error("signature from another key accepted")
	}
	missing := "sha256:" + strings.Repeat("cd", 32)
	if err := v.Verify(ctx, host+"/app@"+missing, pub, Credentials{}); err != ErrNoSignature {
		t.Errorf("unsigned image err = %v, want ErrNoSignature", err)
	}
	if err := v.Verify(ctx, host+"/app:latest", pub, Credentials{}); err == nil {
		t.Error("tag reference must be refused")
	}
}
