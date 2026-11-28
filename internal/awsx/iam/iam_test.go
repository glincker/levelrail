package iam

import (
	"context"
	"strings"
	"testing"

	"github.com/glincker/levelrail/internal/awsx/awsxtest"
)

func TestListAttachedRolePoliciesPages(t *testing.T) {
	c, rec := awsxtest.New(t, func(_ awsxtest.Call, n int) awsxtest.Reply {
		if n == 1 {
			return awsxtest.Reply{Body: `<ListAttachedRolePoliciesResponse><ListAttachedRolePoliciesResult><IsTruncated>true</IsTruncated><Marker>m1</Marker><AttachedPolicies><member><PolicyArn>arn:aws:iam::1:policy/a</PolicyArn></member></AttachedPolicies></ListAttachedRolePoliciesResult></ListAttachedRolePoliciesResponse>`}
		}
		return awsxtest.Reply{Body: `<ListAttachedRolePoliciesResponse><ListAttachedRolePoliciesResult><IsTruncated>false</IsTruncated><AttachedPolicies><member><PolicyArn>arn:aws:iam::1:policy/b</PolicyArn></member></AttachedPolicies></ListAttachedRolePoliciesResult></ListAttachedRolePoliciesResponse>`}
	})
	got, err := New(c).ListAttachedRolePolicies(context.Background(), "lr")
	calls := rec.Calls()
	if err != nil || len(got) != 2 || got[1] != "arn:aws:iam::1:policy/b" || len(calls) != 2 ||
		!strings.Contains(calls[0].Body, "Action=ListAttachedRolePolicies") || !strings.Contains(calls[1].Body, "Marker=m1") {
		t.Fatalf("got %v %v %+v", err, got, calls)
	}
}

func TestRoleName(t *testing.T) {
	if RoleName("arn:aws:iam::123456789012:role/path/lr") != "lr" || RoleName("arn:aws:iam::1:user/x") != "" {
		t.Fatal("role name")
	}
}
