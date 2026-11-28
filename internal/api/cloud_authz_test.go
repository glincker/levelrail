package api

import (
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/store"
)

func cloudTestGrant(kind, ref string, expires *time.Time, actions ...string) store.CloudGrant {
	return store.CloudGrant{AccountID: "oracle-prod", ResourceKind: kind, ResourceRef: ref, Actions: actions, ExpiresAt: expires}
}

func TestAuthorizeCloud(t *testing.T) {
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	past, future := now.Add(-time.Minute), now.Add(time.Hour)
	inst := cloudInstanceResource("oracle-prod", "comp1", "inst1")

	tests := []struct {
		name       string
		in         cloudAuthzInput
		wantAllow  bool
		wantHidden bool
	}{
		{"deny beats root", cloudAuthzInput{Action: store.CloudActionView, Resource: inst, Root: true, Denied: true}, false, false},
		{"deny beats grant", cloudAuthzInput{Action: store.CloudActionView, Resource: inst, Denied: true,
			Grants: []store.CloudGrant{cloudTestGrant(store.CloudResourceAccount, "", nil, store.CloudActionView)}}, false, false},
		{"root passes", cloudAuthzInput{Action: store.CloudActionOperate, Resource: inst, Root: true}, true, false},
		{"root passes build", cloudAuthzInput{Action: cloudActionBuild, Resource: inst, Root: true}, true, false},
		{"account grant reaches instance", cloudAuthzInput{Action: store.CloudActionOperate, Resource: inst,
			Grants: []store.CloudGrant{cloudTestGrant(store.CloudResourceAccount, "", nil, store.CloudActionOperate)}}, true, false},
		{"compartment grant reaches instance", cloudAuthzInput{Action: store.CloudActionView, Resource: inst,
			Grants: []store.CloudGrant{cloudTestGrant(store.CloudResourceCompartment, "comp1", nil, store.CloudActionView)}}, true, false},
		{"other compartment grant does not", cloudAuthzInput{Action: store.CloudActionView, Resource: inst,
			Grants: []store.CloudGrant{cloudTestGrant(store.CloudResourceCompartment, "comp2", nil, store.CloudActionView)}}, false, true},
		{"instance grant does not reach sibling", cloudAuthzInput{Action: store.CloudActionView, Resource: cloudInstanceResource("oracle-prod", "comp1", "inst2"),
			Grants: []store.CloudGrant{cloudTestGrant(store.CloudResourceInstance, "inst1", nil, store.CloudActionView)}}, false, true},
		{"expired grant fails", cloudAuthzInput{Action: store.CloudActionView, Resource: inst, Now: now,
			Grants: []store.CloudGrant{cloudTestGrant(store.CloudResourceAccount, "", &past, store.CloudActionView)}}, false, true},
		{"unexpired grant passes", cloudAuthzInput{Action: store.CloudActionView, Resource: inst, Now: now,
			Grants: []store.CloudGrant{cloudTestGrant(store.CloudResourceAccount, "", &future, store.CloudActionView)}}, true, false},
		{"view grant cannot operate but is visible", cloudAuthzInput{Action: store.CloudActionOperate, Resource: inst, Now: now,
			Grants: []store.CloudGrant{cloudTestGrant(store.CloudResourceAccount, "", nil, store.CloudActionView)}}, false, false},
		{"grant on another account is ignored", cloudAuthzInput{Action: store.CloudActionView, Resource: inst, Now: now,
			Grants: []store.CloudGrant{{AccountID: "other", ResourceKind: store.CloudResourceAccount, Actions: []string{store.CloudActionView}}}}, false, true},
		{"no grant is hidden", cloudAuthzInput{Action: store.CloudActionView, Resource: inst, Now: now}, false, true},
		{"team admin acts as account root", cloudAuthzInput{Action: store.CloudActionManageBudget, Resource: inst, Now: now, OwnerTeamAdmin: true}, true, false},
		{"team admin cannot build", cloudAuthzInput{Action: cloudActionBuild, Resource: inst, Now: now, OwnerTeamAdmin: true}, false, false},
		{"grant cannot delete", cloudAuthzInput{Action: cloudActionDelete, Resource: inst, Now: now,
			Grants: []store.CloudGrant{cloudTestGrant(store.CloudResourceAccount, "", nil, store.CloudActionOperate, store.CloudActionShare)}}, false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.in.Now.IsZero() {
				tc.in.Now = now
			}
			got := authorizeCloud(tc.in)
			if got.Allowed != tc.wantAllow || got.Hidden != tc.wantHidden {
				t.Errorf("authorizeCloud = %+v, want allow=%v hidden=%v", got, tc.wantAllow, tc.wantHidden)
			}
		})
	}
}

func TestAuthorizeCloudGrantNoEscalation(t *testing.T) {
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	future := now.Add(time.Hour)
	acct := cloudAccountResource("oracle-prod")
	comp := cloudCompartmentResource("oracle-prod", "comp1")
	shareView := cloudTestGrant(store.CloudResourceAccount, "", nil, store.CloudActionShare, store.CloudActionView)

	tests := []struct {
		name    string
		in      cloudGrantRequest
		wantErr bool
	}{
		{"root grants anything", cloudGrantRequest{Root: true, Resource: acct, Actions: []string{store.CloudActionShare, store.CloudActionOperate}}, false},
		{"subset passes", cloudGrantRequest{Resource: comp, Actions: []string{store.CloudActionView}, GrantorGrants: []store.CloudGrant{shareView}}, false},
		{"cannot pass on operate", cloudGrantRequest{Resource: comp, Actions: []string{store.CloudActionOperate}, GrantorGrants: []store.CloudGrant{shareView}}, true},
		{"needs share", cloudGrantRequest{Resource: comp, Actions: []string{store.CloudActionView},
			GrantorGrants: []store.CloudGrant{cloudTestGrant(store.CloudResourceAccount, "", nil, store.CloudActionView)}}, true},
		{"compartment share cannot widen to account", cloudGrantRequest{Resource: acct, Actions: []string{store.CloudActionView},
			GrantorGrants: []store.CloudGrant{cloudTestGrant(store.CloudResourceCompartment, "comp1", nil, store.CloudActionShare, store.CloudActionView)}}, true},
		{"build is not grantable", cloudGrantRequest{Resource: comp, Actions: []string{cloudActionBuild}, GrantorGrants: []store.CloudGrant{shareView}}, true},
		{"instance owned account is root only at account level", cloudGrantRequest{Resource: acct, AccountInstanceScoped: true,
			Actions: []string{store.CloudActionView}, GrantorGrants: []store.CloudGrant{shareView}}, true},
		{"team admin grants on a team account", cloudGrantRequest{Resource: acct, OwnerTeamAdmin: true, Actions: []string{store.CloudActionOperate}}, false},
		{"team admin cannot grant on an instance account", cloudGrantRequest{Resource: acct, OwnerTeamAdmin: true, AccountInstanceScoped: true, Actions: []string{store.CloudActionOperate}}, true},
		{"expired share grant is no standing", cloudGrantRequest{Resource: comp, Actions: []string{store.CloudActionView},
			GrantorGrants: []store.CloudGrant{cloudTestGrant(store.CloudResourceAccount, "", &[]time.Time{now.Add(-time.Second)}[0], store.CloudActionShare, store.CloudActionView)}}, true},
		{"cannot outlive grantor", cloudGrantRequest{Resource: comp, Actions: []string{store.CloudActionView},
			GrantorGrants: []store.CloudGrant{cloudTestGrant(store.CloudResourceAccount, "", &future, store.CloudActionShare, store.CloudActionView)}}, true},
		{"bounded grant under bounded grantor", cloudGrantRequest{Resource: comp, Actions: []string{store.CloudActionView}, ExpiresAt: &future,
			GrantorGrants: []store.CloudGrant{cloudTestGrant(store.CloudResourceAccount, "", &future, store.CloudActionShare, store.CloudActionView)}}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.in.Now = now
			if err := authorizeCloudGrant(tc.in); (err != nil) != tc.wantErr {
				t.Errorf("authorizeCloudGrant err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestCloudResourceStrings(t *testing.T) {
	r := cloudInstanceResource("a", "c", "i")
	if r.String() != "cloud:a/instance:i" {
		t.Errorf("instance = %q", r.String())
	}
	if got := r.ancestors(); len(got) != 2 || got[0] != "cloud:a/compartment:c" || got[1] != "cloud:a" {
		t.Errorf("ancestors = %v", got)
	}
	if cloudAccountResource("a").String() != "cloud:a" {
		t.Error("account string")
	}
}
