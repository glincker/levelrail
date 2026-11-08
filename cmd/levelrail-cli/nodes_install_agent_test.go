package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestPrintInstallAgentScript(t *testing.T) {
	var out bytes.Buffer
	printInstallAgentScript(&out, apiclient.CreateNodeJoinTokenResponse{Token: "tok", ControlPlaneAddr: "cp.example.com:9443", CAFingerprint: "ab12"}, installAgentOpts{Name: "oci1", Mesh: true})
	got := out.String()
	for _, want := range []string{"-o install-agent.sh", "--server cp.example.com:9443", "--token-stdin", "--ca-fingerprint ab12", "--name oci1", "--mesh", "printf '%s' 'tok'"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
	if strings.Contains(got, "--token tok") {
		t.Errorf("token must not be a process argument: %q", got)
	}
	out.Reset()
	printInstallAgentScript(&out, apiclient.CreateNodeJoinTokenResponse{Token: "tok"}, installAgentOpts{})
	if out.Len() != 0 {
		t.Errorf("no address must print nothing, got %q", out.String())
	}
}
