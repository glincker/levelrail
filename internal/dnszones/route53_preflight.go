package dnszones

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/aws/smithy-go"
)

// Permission states reported by a Route 53 preflight.
const (
	PermAllowed = "allowed"
	PermDenied  = "denied"
	PermUnknown = "unknown"
)

// Route 53 actions the product uses, in the order a preflight reports them.
const (
	r53ActListZones   = "route53:ListHostedZones"
	r53ActListByName  = "route53:ListHostedZonesByName"
	r53ActGetZone     = "route53:GetHostedZone"
	r53ActListRecords = "route53:ListResourceRecordSets"
	r53ActChange      = "route53:ChangeResourceRecordSets"
	r53ActGetChange   = "route53:GetChange"
)

// PermissionCheck is one probed action. It never carries provider error text.
type PermissionCheck struct {
	Action string `json:"action"`
	State  string `json:"state"`
}

// Route53PreflightAPI is the extra SDK surface a preflight needs beyond Route53API.
type Route53PreflightAPI interface {
	ListHostedZonesByName(ctx context.Context, in *route53.ListHostedZonesByNameInput, opts ...func(*route53.Options)) (*route53.ListHostedZonesByNameOutput, error)
	GetChange(ctx context.Context, in *route53.GetChangeInput, opts ...func(*route53.Options)) (*route53.GetChangeOutput, error)
}

// accessState maps a probe error to a state. Only an authorization code counts
// as denied; a validation style error proves the call was permitted.
func accessState(err error) string {
	if err == nil {
		return PermAllowed
	}
	var ae smithy.APIError
	if errors.As(err, &ae) {
		switch ae.ErrorCode() {
		case "AccessDenied", "AccessDeniedException", "UnauthorizedOperation", "UnrecognizedClientException", "InvalidClientTokenId", "SignatureDoesNotMatch":
			return PermDenied
		case "InvalidChangeBatch", "NoSuchChange", "NoSuchHostedZone", "InvalidInput":
			return PermAllowed
		}
	}
	return PermUnknown
}

// Preflight probes each needed action with a read or a change that cannot
// apply: the write probe deletes a record that does not exist, so it is
// refused by validation when permitted and by authorization when not.
func (r *Route53) Preflight(ctx context.Context) []PermissionCheck {
	var out []PermissionCheck
	add := func(action string, err error) {
		out = append(out, PermissionCheck{Action: action, State: accessState(err)})
	}

	zones, lerr := r.API.ListHostedZones(ctx, &route53.ListHostedZonesInput{MaxItems: aws.Int32(100)})
	add(r53ActListZones, lerr)

	zoneID, zoneName := trimZoneID(r.HostedZoneID), ""
	if lerr == nil {
		for _, hz := range zones.HostedZones {
			if hz.Config != nil && hz.Config.PrivateZone {
				continue
			}
			if zoneID == "" || trimZoneID(aws.ToString(hz.Id)) == zoneID {
				zoneID, zoneName = trimZoneID(aws.ToString(hz.Id)), strings.TrimSuffix(aws.ToString(hz.Name), ".")
				break
			}
		}
	}
	if pa, ok := r.API.(Route53PreflightAPI); ok {
		_, err := pa.ListHostedZonesByName(ctx, &route53.ListHostedZonesByNameInput{MaxItems: aws.Int32(1)})
		add(r53ActListByName, err)
	}
	if zoneID == "" {
		for _, a := range []string{r53ActGetZone, r53ActListRecords, r53ActChange} {
			out = append(out, PermissionCheck{Action: a, State: PermUnknown})
		}
	} else {
		_, err := r.API.GetHostedZone(ctx, &route53.GetHostedZoneInput{Id: aws.String(zoneID)})
		add(r53ActGetZone, err)
		_, err = r.API.ListResourceRecordSets(ctx, &route53.ListResourceRecordSetsInput{HostedZoneId: aws.String(zoneID), MaxItems: aws.Int32(1)})
		add(r53ActListRecords, err)
		if zoneName == "" {
			zoneName = "example.invalid"
		}
		name := "_lr-preflight." + zoneName + "."
		_, err = r.API.ChangeResourceRecordSets(ctx, &route53.ChangeResourceRecordSetsInput{
			HostedZoneId: aws.String(zoneID),
			ChangeBatch: &types.ChangeBatch{Changes: []types.Change{{
				Action: types.ChangeActionDelete,
				ResourceRecordSet: &types.ResourceRecordSet{
					Name: aws.String(name), Type: types.RRTypeTxt, TTL: aws.Int64(60),
					ResourceRecords: []types.ResourceRecord{{Value: aws.String(`"levelrail-preflight-absent"`)}},
				},
			}}},
		})
		add(r53ActChange, err)
	}
	if pa, ok := r.API.(Route53PreflightAPI); ok {
		_, err := pa.GetChange(ctx, &route53.GetChangeInput{Id: aws.String("C00000000000000000000")})
		add(r53ActGetChange, err)
	}
	return out
}

// MissingPermissions names the denied actions in probe order.
func MissingPermissions(checks []PermissionCheck) []string {
	var miss []string
	for _, c := range checks {
		if c.State == PermDenied {
			miss = append(miss, c.Action)
		}
	}
	return miss
}

// MinimalIAMPolicy is the smallest policy JSON granting the missing actions,
// scoped to the zone where the action allows it.
func MinimalIAMPolicy(missing []string, zoneID string) string {
	if len(missing) == 0 {
		return ""
	}
	zoneRes := "*"
	if zoneID != "" {
		zoneRes = "arn:aws:route53:::hostedzone/" + trimZoneID(zoneID)
	}
	var global, scoped []string
	for _, a := range missing {
		switch a {
		case r53ActGetZone, r53ActListRecords, r53ActChange:
			scoped = append(scoped, a)
		default:
			global = append(global, a)
		}
	}
	type stmt struct {
		Effect   string   `json:"Effect"`
		Action   []string `json:"Action"`
		Resource string   `json:"Resource"`
	}
	var sts []stmt
	sort.Strings(global)
	sort.Strings(scoped)
	if len(global) > 0 {
		sts = append(sts, stmt{"Allow", global, "*"})
	}
	if len(scoped) > 0 {
		sts = append(sts, stmt{"Allow", scoped, zoneRes})
	}
	b, _ := json.MarshalIndent(map[string]any{"Version": "2012-10-17", "Statement": sts}, "", "  ")
	return string(b)
}
