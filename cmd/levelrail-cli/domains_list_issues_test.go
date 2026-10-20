package main

import (
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestRun_DomainsList_ShowsIngressIssue(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, []domainResource{{
		Domain: "app.example.com", ServiceName: "web",
		Issues: []apiclient.DomainIssue{{Domain: "app.example.com", Code: "basic_auth_unresolved", Severity: "dropped", Message: "password unreadable"}},
	}})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"domains", "list", "--api-url", srv.URL})
	if !strings.Contains(stdout, "dropped: password unreadable") {
		t.Errorf("stdout = %q, want the ingress issue shown", stdout)
	}
}
