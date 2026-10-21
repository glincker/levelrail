package ingress

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDiagnosticRoutesSelectionAndEscaping(t *testing.T) {
	routes := diagnosticRoutes(404, `Acme<script>`, `https://d.example/x"y`)
	if len(routes) != 2 || routes[0].Match[0].Header["Accept"][0] != "*text/html*" || routes[1].Match != nil {
		t.Fatalf("html route must precede the plain fallback: %+v", routes)
	}
	htmlResp := routes[0].Handle[0].(StaticResponseHandler)
	plain := routes[1].Handle[0].(StaticResponseHandler)
	if htmlResp.StatusCode != 404 || plain.StatusCode != 404 {
		t.Fatal("status must stay 404")
	}
	if strings.Contains(htmlResp.Body, "<script>Acme") || strings.Contains(htmlResp.Body, `x"y`) || !strings.Contains(htmlResp.Body, "Acme&lt;script&gt;") {
		t.Errorf("brand and docs URL must be escaped: %s", htmlResp.Body)
	}
	if strings.Contains(htmlResp.Body, "{http.request.host}") {
		t.Error("html must not interpolate the host server side")
	}
	if !strings.Contains(plain.Body, "No app is serving this address") || plain.Headers["Content-Type"][0] != "text/plain; charset=utf-8" {
		t.Errorf("plain response wrong: %+v", plain)
	}
	if _, err := json.Marshal(routes); err != nil {
		t.Fatal(err)
	}
}
