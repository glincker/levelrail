package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/compose"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

type memComposeSecrets map[string]string

func (m memComposeSecrets) Resolve(_ context.Context, svc, key string) (string, error) {
	if v, ok := m[svc+"|"+key]; ok {
		return v, nil
	}
	return "", secrets.ErrValueNotFound
}

func (m memComposeSecrets) SetValue(_ context.Context, svc, key, v string) error {
	m[svc+"|"+key] = v
	return nil
}

func secretSvc(content string) []store.DesiredService {
	return []store.DesiredService{{Name: "a-app", Files: []store.ServiceFile{{Path: "/run/secrets/k", Secret: true, NoExpand: true, Content: content}}}}
}

func TestComposeSecretFileRevisionsKeepValue(t *testing.T) {
	mem := memComposeSecrets{}
	rt := &Router{composeSecrets: mem}
	r := httptest.NewRequest(http.MethodPost, "/", nil)

	// Revision 1 stores the body; revision 2 and 3 are redacted replays.
	svcs := secretSvc("pa$$ ${X}")
	if !rt.storeComposeSecretFiles(httptest.NewRecorder(), r, svcs) {
		t.Fatal("first deploy refused")
	}
	if svcs[0].Files[0].Content != "" || svcs[0].Files[0].Hash == "" {
		t.Fatalf("content not blanked: %+v", svcs[0].Files[0])
	}
	first := svcs[0].Files[0].Hash
	for range 2 {
		svcs = secretSvc(compose.RedactedSecretMarker)
		if !rt.storeComposeSecretFiles(httptest.NewRecorder(), r, svcs) {
			t.Fatal("replay refused")
		}
		if svcs[0].Files[0].Hash != first {
			t.Fatal("replay changed the hash")
		}
	}
	if got := mem["a-app|"+store.FileSecretKey("/run/secrets/k")]; got != "pa$$ ${X}" {
		t.Fatalf("stored value = %q", got)
	}
}

func TestComposeSecretFileRefusesWithoutStoredValue(t *testing.T) {
	rt := &Router{composeSecrets: memComposeSecrets{}}
	rec := httptest.NewRecorder()
	if rt.storeComposeSecretFiles(rec, httptest.NewRequest(http.MethodPost, "/", nil), secretSvc(compose.RedactedSecretMarker)) {
		t.Fatal("want refusal")
	}
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d", rec.Code)
	}
}
