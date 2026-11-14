package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/iac"
)

func TestStrictIaCComposeServerSide(t *testing.T) {
	t.Cleanup(func() { iac.SetComposeSource(func() iac.ComposeLimits { return iac.ComposeLimits{} }) })
	iac.SetComposeSource(func() iac.ComposeLimits { return iac.ComposeLimits{} })
	w := httptest.NewRecorder()
	if strictIaCCompose(w, []byte("services:\n  web: {image: nginx}\n"), nil) || w.Code != http.StatusConflict {
		t.Fatalf("gate off must refuse, got %d", w.Code)
	}
	iac.SetComposeSource(func() iac.ComposeLimits { return iac.ComposeLimits{Enabled: true} })
	w = httptest.NewRecorder()
	if strictIaCCompose(w, []byte("services:\n  web:\n    image: nginx\n    privileged: true\n"), nil) || !strings.Contains(w.Body.String(), iac.CodeComposeRejected) {
		t.Fatalf("privileged must be refused with a code: %s", w.Body.String())
	}
	w = httptest.NewRecorder()
	if !strictIaCCompose(w, []byte("services:\n  web:\n    image: nginx:${TAG}\n"), map[string]string{"TAG": "1.27"}) {
		t.Fatalf("a declared var must pass: %s", w.Body.String())
	}
}
