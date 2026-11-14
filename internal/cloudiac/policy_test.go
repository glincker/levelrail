package cloudiac

import "testing"

func codes(rs []Refusal) []string {
	var out []string
	for _, r := range rs {
		out = append(out, r.Code)
	}
	return out
}

func TestPolicyCheck(t *testing.T) {
	cases := []struct {
		name    string
		p       Policy
		changes []ResourceChange
		want    []string
	}{
		{"clean create", Policy{}, []ResourceChange{{"a", "oci_core_vcn", ActionCreate}}, nil},
		{"noop iam ignored", Policy{}, []ResourceChange{{"a", "aws_iam_role", ActionNoop}}, nil},
		{"oci policy", Policy{}, []ResourceChange{{"a", "oci_identity_policy", ActionCreate}}, []string{CodeIAMForbidden}},
		{"aws role update", Policy{}, []ResourceChange{{"a", "aws_iam_role", ActionUpdate}}, []string{CodeIAMForbidden}},
		{"aws attachment prefix", Policy{}, []ResourceChange{{"a", "aws_iam_role_policy_attachment", ActionCreate}}, []string{CodeIAMForbidden}},
		{"bucket delete", Policy{MaxDestroys: 5}, []ResourceChange{{"b", "oci_objectstorage_bucket", ActionDelete}}, []string{CodeProtectedDelete}},
		{"bucket replace", Policy{MaxDestroys: 5}, []ResourceChange{{"b", "aws_s3_bucket", ActionReplace}}, []string{CodeProtectedDelete}},
		{"bucket delete opted in", Policy{MaxDestroys: 5, AllowProtectedDelete: true}, []ResourceChange{{"b", "aws_s3_bucket", ActionDelete}}, nil},
		{"default cap zero", Policy{}, []ResourceChange{{"n", "oci_core_vcn", ActionDelete}}, []string{CodeDestroyCap}},
		{"cap allows one", Policy{MaxDestroys: 1}, []ResourceChange{{"n", "oci_core_vcn", ActionDelete}}, nil},
		{"cap exceeded", Policy{MaxDestroys: 1}, []ResourceChange{{"n", "oci_core_vcn", ActionDelete}, {"m", "oci_core_subnet", ActionReplace}}, []string{CodeDestroyCap}},
		{"volume delete under zero cap", Policy{}, []ResourceChange{{"v", "oci_core_volume", ActionDelete}}, []string{CodeDestroyCap, CodeProtectedDelete}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := codes(tc.p.Check(tc.changes))
			if len(got) != len(tc.want) {
				t.Fatalf("got %v want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("got %v want %v", got, tc.want)
				}
			}
		})
	}
}

func TestNewPolicy(t *testing.T) {
	if p := NewPolicy(3, true); p.MaxDestroys != 3 || !p.AllowProtectedDelete {
		t.Fatalf("got %+v", p)
	}
	if p := NewPolicy(-2, false); p.MaxDestroys != 0 || p.AllowProtectedDelete {
		t.Fatalf("negative cap must stay strict: %+v", p)
	}
}

func TestRefusalsError(t *testing.T) {
	if RefusalsError(nil) != nil {
		t.Fatal("expected nil")
	}
	if RefusalsError([]Refusal{{Code: CodeDestroyCap, Message: "x"}}) == nil {
		t.Fatal("expected error")
	}
}
