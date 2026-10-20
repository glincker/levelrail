package dnszones

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/route53/types"
)

func (r *Route53) rawSets(ctx context.Context, zone Zone, start *Key) ([]types.ResourceRecordSet, error) {
	in := &route53.ListResourceRecordSetsInput{HostedZoneId: aws.String(trimZoneID(zone.ID))}
	if start != nil {
		in.StartRecordName = aws.String(FQDN(start.Name, zone.Name) + ".")
		in.StartRecordType = types.RRType(start.Type)
		in.MaxItems = aws.Int32(100)
	}
	var all []types.ResourceRecordSet
	for {
		out, err := r.API.ListResourceRecordSets(ctx, in)
		if err != nil {
			return nil, fmt.Errorf("route53: list record sets: %w", err)
		}
		all = append(all, out.ResourceRecordSets...)
		if !out.IsTruncated || start != nil {
			return all, nil
		}
		in.StartRecordName, in.StartRecordType, in.StartRecordIdentifier = out.NextRecordName, out.NextRecordType, out.NextRecordIdentifier
	}
}

func fromR53(s types.ResourceRecordSet, zone string) RecordSet {
	rs := RecordSet{
		Name:          RelativeName(unescapeOctal(aws.ToString(s.Name)), zone),
		Type:          string(s.Type),
		TTL:           int(aws.ToInt64(s.TTL)),
		SetIdentifier: aws.ToString(s.SetIdentifier),
		HealthCheckID: aws.ToString(s.HealthCheckId),
		Weight:        s.Weight,
		Failover:      string(s.Failover),
		Routing:       RoutingSimple,
	}
	switch {
	case s.Weight != nil:
		rs.Routing = RoutingWeighted
	case s.Failover != "":
		rs.Routing = RoutingFailover
	case aws.ToBool(s.MultiValueAnswer):
		rs.Routing = RoutingMultivalue
	case s.Region != "":
		rs.Routing = "latency"
	case s.GeoLocation != nil || s.GeoProximityLocation != nil || s.CidrRoutingConfig != nil:
		rs.Routing = "geolocation"
	}
	if s.AliasTarget != nil {
		rs.Alias = &Alias{DNSName: NormalizeDomain(aws.ToString(s.AliasTarget.DNSName)), HostedZoneID: aws.ToString(s.AliasTarget.HostedZoneId), EvaluateTargetHealth: s.AliasTarget.EvaluateTargetHealth}
	}
	for _, v := range s.ResourceRecords {
		val := aws.ToString(v.Value)
		switch rs.Type {
		case "TXT":
			val = unquoteTXT(val)
		case "CNAME", "NS":
			val = NormalizeDomain(val)
		case "MX":
			val = absLastFieldTrim(val, 1)
		case "SRV":
			val = absLastFieldTrim(val, 3)
		}
		rs.Values = append(rs.Values, val)
	}
	return rs
}

func absLastFieldTrim(v string, idx int) string {
	f := strings.Fields(v)
	if len(f) == idx+1 {
		f[idx] = NormalizeDomain(f[idx])
	}
	return strings.Join(f, " ")
}

// ListRecordSets implements Provider (ListResourceRecordSets, paginated).
func (r *Route53) ListRecordSets(ctx context.Context, zone Zone) ([]RecordSet, error) {
	raw, err := r.rawSets(ctx, zone, nil)
	if err != nil {
		return nil, err
	}
	out := make([]RecordSet, 0, len(raw))
	for _, s := range raw {
		out = append(out, fromR53(s, zone.Name))
	}
	SortSets(out)
	return out, nil
}

func toR53(rs RecordSet, zone Zone) types.ResourceRecordSet {
	ttl := int64(rs.TTL)
	s := types.ResourceRecordSet{Name: aws.String(FQDN(rs.Name, zone.Name) + "."), Type: types.RRType(rs.Type), TTL: &ttl}
	for _, v := range rs.Values {
		if rs.Type == "TXT" {
			v = quoteTXT(v)
		}
		s.ResourceRecords = append(s.ResourceRecords, types.ResourceRecord{Value: aws.String(v)})
	}
	if rs.SetIdentifier != "" {
		s.SetIdentifier = aws.String(rs.SetIdentifier)
	}
	if rs.HealthCheckID != "" {
		s.HealthCheckId = aws.String(rs.HealthCheckID)
	}
	switch rs.Routing {
	case RoutingWeighted:
		s.Weight = rs.Weight
	case RoutingFailover:
		s.Failover = types.ResourceRecordSetFailover(rs.Failover)
	case RoutingMultivalue:
		s.MultiValueAnswer = aws.Bool(true)
	}
	return s
}

func (r *Route53) change(ctx context.Context, zone Zone, action types.ChangeAction, s types.ResourceRecordSet) error {
	_, err := r.API.ChangeResourceRecordSets(ctx, &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(trimZoneID(zone.ID)),
		ChangeBatch: &types.ChangeBatch{
			Comment: aws.String("levelrail " + time.Now().UTC().Format(time.RFC3339)),
			Changes: []types.Change{{Action: action, ResourceRecordSet: &s}},
		},
	})
	if err != nil {
		return fmt.Errorf("route53: %s record set: %w", strings.ToLower(string(action)), err)
	}
	return nil
}

// UpsertRecordSet implements Provider (ChangeResourceRecordSets UPSERT).
func (r *Route53) UpsertRecordSet(ctx context.Context, zone Zone, rs RecordSet) error {
	return r.change(ctx, zone, types.ChangeActionUpsert, toR53(rs, zone))
}

// DeleteRecordSet implements Provider. Route53's DELETE needs the set's exact
// current content, so it is read back first; a missing set is not an error.
func (r *Route53) DeleteRecordSet(ctx context.Context, zone Zone, k Key) error {
	raw, err := r.rawSets(ctx, zone, &k)
	if err != nil {
		return err
	}
	for _, s := range raw {
		if fromR53(s, zone.Name).Key() == k {
			return r.change(ctx, zone, types.ChangeActionDelete, s)
		}
	}
	return nil
}

// ListHealthChecks implements HealthChecker.
func (r *Route53) ListHealthChecks(ctx context.Context) ([]HealthCheck, error) {
	var out []HealthCheck
	var marker *string
	for {
		res, err := r.API.ListHealthChecks(ctx, &route53.ListHealthChecksInput{Marker: marker})
		if err != nil {
			return nil, fmt.Errorf("route53: list health checks: %w", err)
		}
		for _, hc := range res.HealthChecks {
			h := HealthCheck{ID: aws.ToString(hc.Id)}
			if c := hc.HealthCheckConfig; c != nil {
				h.Type, h.IPAddress, h.FQDN = string(c.Type), aws.ToString(c.IPAddress), aws.ToString(c.FullyQualifiedDomainName)
				h.Port, h.ResourcePath = aws.ToInt32(c.Port), aws.ToString(c.ResourcePath)
				h.RequestInterval, h.FailureThreshold = aws.ToInt32(c.RequestInterval), aws.ToInt32(c.FailureThreshold)
			}
			out = append(out, h)
		}
		if !res.IsTruncated || res.NextMarker == nil {
			return out, nil
		}
		marker = res.NextMarker
	}
}

// CreateHealthCheck implements HealthChecker (HTTP, HTTPS or TCP).
func (r *Route53) CreateHealthCheck(ctx context.Context, hc HealthCheck) (HealthCheck, error) {
	cfg := &types.HealthCheckConfig{Type: types.HealthCheckType(strings.ToUpper(hc.Type))}
	if hc.IPAddress != "" {
		cfg.IPAddress = aws.String(hc.IPAddress)
	}
	if hc.FQDN != "" {
		cfg.FullyQualifiedDomainName = aws.String(hc.FQDN)
	}
	if hc.Port > 0 {
		cfg.Port = aws.Int32(hc.Port)
	}
	if hc.ResourcePath != "" && cfg.Type != types.HealthCheckTypeTcp {
		cfg.ResourcePath = aws.String(hc.ResourcePath)
	}
	if hc.RequestInterval > 0 {
		cfg.RequestInterval = aws.Int32(hc.RequestInterval)
	}
	if hc.FailureThreshold > 0 {
		cfg.FailureThreshold = aws.Int32(hc.FailureThreshold)
	}
	ref := fmt.Sprintf("levelrail-%d", time.Now().UnixNano())
	out, err := r.API.CreateHealthCheck(ctx, &route53.CreateHealthCheckInput{CallerReference: aws.String(ref), HealthCheckConfig: cfg})
	if err != nil {
		return HealthCheck{}, fmt.Errorf("route53: create health check: %w", err)
	}
	hc.ID = aws.ToString(out.HealthCheck.Id)
	return hc, nil
}

// DeleteHealthCheck implements HealthChecker.
func (r *Route53) DeleteHealthCheck(ctx context.Context, id string) error {
	if _, err := r.API.DeleteHealthCheck(ctx, &route53.DeleteHealthCheckInput{HealthCheckId: aws.String(id)}); err != nil {
		return fmt.Errorf("route53: delete health check: %w", err)
	}
	return nil
}
