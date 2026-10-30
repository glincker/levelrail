package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/catalog"
)

// Go batch templates live in internal/catalog/templates_catalog_batch*.go, not in
// manifests/, so their description path differs from the manifest-backed one.
func TestHandleServiceTemplate_GoBatchDescription(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	var rustdesk catalog.Template
	for _, tpl := range catalog.Templates {
		if tpl.ID == "rustdesk-server" {
			rustdesk = tpl
			break
		}
	}
	if rustdesk.ID == "" || rustdesk.Manifest != nil {
		t.Fatal("rustdesk-server must be a Go batch template without a manifest")
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/service-templates/rustdesk-server", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("detail status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var detail struct {
		Description string `json:"description"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decode detail: %v", err)
	}
	if detail.Description == "" {
		t.Fatal("detail description is empty, want the Go batch description")
	}
	if !strings.Contains(detail.Description, "RUSTDESK_RELAY") {
		t.Errorf("detail description %q does not mention RUSTDESK_RELAY", detail.Description)
	}
	for _, port := range []string{"21115-21117", "21116"} {
		if !strings.Contains(detail.Description, port) {
			t.Errorf("detail description does not mention firewall port %s", port)
		}
	}
	if detail.Description != rustdesk.Description {
		t.Errorf("detail description = %q, want catalog value %q", detail.Description, rustdesk.Description)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/service-templates", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var list []serviceTemplateListItem
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	found := false
	for _, item := range list {
		if item.ID == "rustdesk-server" {
			found = true
			if item.Description != rustdesk.Description {
				t.Errorf("list description = %q, want catalog value %q", item.Description, rustdesk.Description)
			}
		}
	}
	if !found {
		t.Fatal("rustdesk-server not present in list response")
	}
}

// The description is served as plain text: newlines stay literal and no markup
// is added or rendered, so the dashboard can wrap it itself.
func TestHandleServiceTemplate_GoBatchDescriptionIsPlainText(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/service-templates/mosquitto", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got struct {
		Description string `json:"description"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	var want string
	for _, tpl := range catalog.Templates {
		if tpl.ID == "mosquitto" {
			want = tpl.Description
		}
	}
	if got.Description != want {
		t.Errorf("description = %q, want raw catalog value %q", got.Description, want)
	}
	if !strings.Contains(got.Description, "\n") {
		t.Error("description lost its line breaks")
	}
	for _, markup := range []string{"<", ">", "**", "](", "&amp;", "&lt;"} {
		if strings.Contains(got.Description, markup) {
			t.Errorf("description contains markup %q, want plain text", markup)
		}
	}
}

// A Go template with no description must not get an invented one, and the
// field is omitted from the detail body.
func TestHandleServiceTemplate_GoBatchWithoutDescriptionOmitsField(t *testing.T) {
	var target catalog.Template
	for _, tpl := range catalog.Templates {
		if tpl.Manifest == nil && tpl.Description == "" {
			target = tpl
			break
		}
	}
	if target.ID == "" {
		t.Skip("every Go batch template has a description")
	}

	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/service-templates/"+target.ID, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"description"`) {
		t.Errorf("detail for %q includes a description field, want it omitted", target.ID)
	}
}
