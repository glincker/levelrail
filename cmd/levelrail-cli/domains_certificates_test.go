package main

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestRun_DomainsCertificates_List(t *testing.T) {
	var gotPath string
	notAfter := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	srv := newListEchoServer(t, &gotPath, []certificateResource{
		{Domain: "example.com", Issuer: "Let's Encrypt", Status: "healthy", NotAfter: notAfter},
	})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"domains", "certificates", "--api-url", srv.URL})

	if gotPath != "/api/v1/certificates" {
		t.Errorf("path = %s, want /api/v1/certificates", gotPath)
	}
	if !strings.Contains(stdout, "example.com") || !strings.Contains(stdout, "healthy") {
		t.Errorf("stdout = %q, want example.com/healthy listed", stdout)
	}
}

func TestRun_DomainsCertificates_List_Empty(t *testing.T) {
	srv := newListEchoServer(t, nil, []certificateResource{})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"domains", "certificates", "--api-url", srv.URL})
	if !strings.Contains(stdout, "no certificates") {
		t.Errorf("stdout = %q, want the empty-set message", stdout)
	}
}

func TestRun_DomainsCertificates_List_JSON(t *testing.T) {
	srv := newListEchoServer(t, nil, []certificateResource{{Domain: "example.com", Status: "healthy"}})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"domains", "certificates", "--json", "--api-url", srv.URL})
	if !strings.Contains(stdout, `"domain":"example.com"`) && !strings.Contains(stdout, `"domain": "example.com"`) {
		t.Errorf("stdout = %q, want JSON containing example.com", stdout)
	}
}

func TestRun_DomainsCertificates_List_APIError(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusInternalServerError, `{"error":"internal error"}`)

	stderr := runCLIExpectAPIError(t, []string{"domains", "certificates", "--api-url", srv.URL})
	if !strings.Contains(stderr, "internal error") {
		t.Errorf("stderr = %q, want the server's error", stderr)
	}
}
