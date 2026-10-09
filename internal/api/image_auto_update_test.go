package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	oldDigest = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	newDigest = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
)

func TestCheckImageUpdate(t *testing.T) {
	tests := []struct {
		name     string
		svc      store.DesiredService
		suspend  bool
		remote   string
		wantHas  string
		wantNewD bool
	}{
		{"same digest is up to date", store.DesiredService{Name: "web", Image: "nginx:1@" + oldDigest, Port: 80}, false, oldDigest, imageUpdateUpToDate, false},
		{"moved tag redeploys", store.DesiredService{Name: "web", Image: "nginx:1@" + oldDigest, Port: 80}, false, newDigest, imageUpdateRedeployed, true},
		{"built from source is skipped", store.DesiredService{Name: "web", Image: "web:abc", ImageIDRef: "web:abc", ImageID: "sha256:aa", Port: 80}, false, newDigest, "built from source", false},
		{"stopped app is skipped", store.DesiredService{Name: "web", Image: "nginx:1@" + oldDigest, Port: 80}, true, newDigest, "app is stopped", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rt, db, _ := newSafetyRouter(t, stubResolver{digest: tc.remote})
			ctx := context.Background()
			if err := db.SaveDesiredService(ctx, tc.svc); err != nil {
				t.Fatal(err)
			}
			if tc.suspend {
				if err := db.UpdateServiceSuspended(ctx, "web", true); err != nil {
					t.Fatal(err)
				}
			}
			got, err := rt.CheckImageUpdate(ctx, "web")
			if err != nil || !strings.Contains(got, tc.wantHas) {
				t.Fatalf("result = %q, %v; want %q", got, err, tc.wantHas)
			}
			after, err := db.GetDesiredService(ctx, "web")
			if err != nil {
				t.Fatal(err)
			}
			if moved := docker.ImageDigestOf(after.Image) == newDigest; moved != tc.wantNewD {
				t.Errorf("image = %q, moved to new digest = %v, want %v", after.Image, moved, tc.wantNewD)
			}
			rec, err := db.GetImageAutoUpdate(ctx, "web")
			if err != nil || rec.LastResult != got || rec.LastCheckedAt == nil {
				t.Errorf("recorded = %+v, %v; want last result %q", rec, err, got)
			}
		})
	}
}

func TestCheckImageUpdate_UnknownApp(t *testing.T) {
	rt, _, _ := newSafetyRouter(t, stubResolver{digest: newDigest})
	if _, err := rt.CheckImageUpdate(context.Background(), "ghost"); err == nil {
		t.Fatal("want a not-found error")
	}
}

func TestLoggablePath_RedactsWebhookToken(t *testing.T) {
	if got := loggablePath("/api/v1/hooks/image-update/web/s3cret"); strings.Contains(got, "s3cret") || got != "/api/v1/hooks/image-update/web/redacted" {
		t.Errorf("loggablePath = %q", got)
	}
	if got := loggablePath("/api/v1/apps/web"); got != "/api/v1/apps/web" {
		t.Errorf("unrelated path changed: %q", got)
	}
}

func TestImageUpdateWebhook(t *testing.T) {
	rt, db, cookie := newSafetyRouter(t, stubResolver{digest: newDigest})
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "nginx:1@" + oldDigest, Port: 80}); err != nil {
		t.Fatal(err)
	}
	post := func(path string) int {
		rec := serve(rt, authedRequest(t, cookie, "POST", path, ""))
		return rec.Code
	}
	if post("/api/v1/apps/web/auto-update/webhook") != 200 {
		t.Fatal("rotate must succeed")
	}
	rec := serve(rt, authedRequest(t, cookie, "POST", "/api/v1/apps/web/auto-update/webhook", ""))
	var hook imageUpdateWebhookResource
	if err := json.Unmarshal(rec.Body.Bytes(), &hook); err != nil || hook.Token == "" {
		t.Fatalf("rotate body = %s, %v", rec.Body.String(), err)
	}
	if code := post(hook.Path); code != 404 {
		t.Errorf("webhook before opt-in = %d, want 404", code)
	}
	if err := db.SetImageAutoUpdate(ctx, "web", true); err != nil {
		t.Fatal(err)
	}
	if code := post("/api/v1/hooks/image-update/web/wrong"); code != 404 {
		t.Errorf("wrong token = %d, want 404", code)
	}
	if code := post(hook.Path); code != 202 {
		t.Errorf("valid webhook = %d, want 202", code)
	}
}
