package api

import (
	"bytes"
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestHandleImportSAMLMetadata(t *testing.T) {
	rt, st := newTestRouter(t)

	// Create a dummy SAML connection in the DB
	conn := store.SSOConnection{
		ID:        "saml_test123",
		Name:      "test",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := st.SaveSSOConnection(context.Background(), conn, true, time.Now()); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// This URL shouldn't be reachable, but using example.com
	reqBody := `{"url": "https://example.com/metadata"}`
	req := httptest.NewRequest("POST", "/api/v1/sso/saml/saml_test123/import", bytes.NewBufferString(reqBody))
	req.SetPathValue("id", "saml_test123")
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()

	// This will hit the route which invokes authengine.FetchSAMLIdPMetadata with netguard.NewClient()
	// and will likely fail with StatusBadGateway because example.com doesn't serve valid SAML, or netguard blocks it if not configured
	rt.handleImportSAMLMetadata(w, req)

	// Just need to assert it ran without panicking and returned an HTTP error
	if w.Code == 200 {
		t.Errorf("expected error code, got %v", w.Code)
	}
}
