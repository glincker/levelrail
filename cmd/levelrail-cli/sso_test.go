package main

import (
	"flag"
	"reflect"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestParseSSOMapping(t *testing.T) {
	for _, tc := range []struct {
		in      string
		group   string
		role    string
		prio    int
		wantErr bool
	}{
		{"Admins=role_admin:100", "Admins", "role_admin", 100, false},
		{"eng = role_operator", "eng", "role_operator", 0, false},
		{"eng=role_x", "eng", "role_x", 0, false},
		{"eng=role_x:high", "", "", 0, true},
		{"=role_x", "", "", 0, true},
		{"eng=", "", "", 0, true},
		{"no-equals", "", "", 0, true},
	} {
		m, err := parseSSOMapping(tc.in)
		if (err != nil) != tc.wantErr {
			t.Fatalf("%q: err %v", tc.in, err)
		}
		if !tc.wantErr && (m.Group != tc.group || m.RoleID != tc.role || m.Priority != tc.prio) {
			t.Fatalf("%q: got %+v", tc.in, m)
		}
	}
}

func TestOIDCPolicyFlagsOverwriteOnlyGivenFlags(t *testing.T) {
	var f oidcPolicyFlags
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	f.register(fs)
	if err := fs.Parse([]string{"--jit=false", "--allowed-domains", "a.test, b.test"}); err != nil {
		t.Fatal(err)
	}
	req := apiclient.OAuthSSOPolicyRequest{GroupsClaim: "roles", SyncRoles: true, DefaultRoleID: "role_operator", JITProvisioning: true}
	f.apply(fs, &req)
	if req.JITProvisioning || !req.SyncRoles || req.GroupsClaim != "roles" || req.DefaultRoleID != "role_operator" {
		t.Errorf("only the given flags may change the policy: %+v", req)
	}
	if !reflect.DeepEqual(req.AllowedDomains, []string{"a.test", "b.test"}) {
		t.Errorf("domains = %v", req.AllowedDomains)
	}
}
