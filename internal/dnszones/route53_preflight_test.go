package dnszones

import (
	"context"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/aws/smithy-go"
)

// pfFake denies the named actions with an AccessDenied code and refuses the
// no-op delete with a validation code, as the real service does.
type pfFake struct {
	Route53API
	deny map[string]bool
}

func (f *pfFake) err(action string) error {
	if f.deny[action] {
		return &smithy.GenericAPIError{Code: "AccessDenied", Message: "x"}
	}
	return nil
}

func (f *pfFake) ListHostedZones(context.Context, *route53.ListHostedZonesInput, ...func(*route53.Options)) (*route53.ListHostedZonesOutput, error) {
	if err := f.err(r53ActListZones); err != nil {
		return nil, err
	}
	return &route53.ListHostedZonesOutput{HostedZones: []types.HostedZone{{Id: aws.String("/hostedzone/Z1"), Name: aws.String("apps.example.com.")}}}, nil
}

func (f *pfFake) ListHostedZonesByName(context.Context, *route53.ListHostedZonesByNameInput, ...func(*route53.Options)) (*route53.ListHostedZonesByNameOutput, error) {
	return &route53.ListHostedZonesByNameOutput{}, f.err(r53ActListByName)
}

func (f *pfFake) GetHostedZone(context.Context, *route53.GetHostedZoneInput, ...func(*route53.Options)) (*route53.GetHostedZoneOutput, error) {
	return &route53.GetHostedZoneOutput{}, f.err(r53ActGetZone)
}

func (f *pfFake) ListResourceRecordSets(context.Context, *route53.ListResourceRecordSetsInput, ...func(*route53.Options)) (*route53.ListResourceRecordSetsOutput, error) {
	return &route53.ListResourceRecordSetsOutput{}, f.err(r53ActListRecords)
}

func (f *pfFake) ChangeResourceRecordSets(context.Context, *route53.ChangeResourceRecordSetsInput, ...func(*route53.Options)) (*route53.ChangeResourceRecordSetsOutput, error) {
	if err := f.err(r53ActChange); err != nil {
		return nil, err
	}
	return nil, &smithy.GenericAPIError{Code: "InvalidChangeBatch", Message: "not found"}
}

func (f *pfFake) GetChange(context.Context, *route53.GetChangeInput, ...func(*route53.Options)) (*route53.GetChangeOutput, error) {
	if err := f.err(r53ActGetChange); err != nil {
		return nil, err
	}
	return nil, &smithy.GenericAPIError{Code: "NoSuchChange", Message: "x"}
}

func TestRoute53Preflight(t *testing.T) {
	tests := []struct {
		name string
		deny []string
		want []string
	}{
		{"all allowed", nil, nil},
		{"write and change lookup denied", []string{r53ActChange, r53ActGetChange}, []string{r53ActChange, r53ActGetChange}},
		{"read only", []string{r53ActChange}, []string{r53ActChange}},
		{"list denied leaves zone probes unknown", []string{r53ActListZones}, []string{r53ActListZones}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := &pfFake{deny: map[string]bool{}}
			for _, d := range tc.deny {
				f.deny[d] = true
			}
			r := &Route53{API: f, HostedZoneID: "Z1"}
			got := MissingPermissions(r.Preflight(context.Background()))
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Fatalf("missing = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestMinimalIAMPolicy(t *testing.T) {
	if MinimalIAMPolicy(nil, "Z1") != "" {
		t.Fatal("no missing actions must give no policy")
	}
	p := MinimalIAMPolicy([]string{r53ActChange, r53ActListZones}, "/hostedzone/Z1")
	for _, want := range []string{r53ActChange, r53ActListZones, "arn:aws:route53:::hostedzone/Z1", `"Resource": "*"`} {
		if !strings.Contains(p, want) {
			t.Errorf("policy lacks %q:\n%s", want, p)
		}
	}
}
