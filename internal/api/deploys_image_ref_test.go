package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestHandleTriggerDeploy_InvalidImageReferenceNamesTheField(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	for _, image := range []string{"Bad Image", "team/Case:1", "web:", "reg.example.com//x"} {
		body, err := json.Marshal(map[string]string{"image": image})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/deploys", string(body)))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("image %q: status = %d, want 400; body = %s", image, rec.Code, rec.Body.String())
		}
		var got codedError
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || got.Code != errCodeInvalidImageRef || !strings.HasPrefix(got.Error, "image:") {
			t.Errorf("image %q: body = %s, want code %s and a message naming the image field", image, rec.Body.String(), errCodeInvalidImageRef)
		}
	}
	svc, err := db.GetDesiredService(context.Background(), "web")
	if err != nil || svc.Image != "levelrail/web:1" {
		t.Errorf("a rejected image must not change desired state: %+v, %v", svc, err)
	}
}
