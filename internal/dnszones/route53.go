package dnszones

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/route53/types"
)

// Route53API is the subset of the AWS SDK client this package calls, so tests use a fake.
type Route53API interface {
	ListHostedZones(ctx context.Context, in *route53.ListHostedZonesInput, opts ...func(*route53.Options)) (*route53.ListHostedZonesOutput, error)
	GetHostedZone(ctx context.Context, in *route53.GetHostedZoneInput, opts ...func(*route53.Options)) (*route53.GetHostedZoneOutput, error)
	CreateHostedZone(ctx context.Context, in *route53.CreateHostedZoneInput, opts ...func(*route53.Options)) (*route53.CreateHostedZoneOutput, error)
	DeleteHostedZone(ctx context.Context, in *route53.DeleteHostedZoneInput, opts ...func(*route53.Options)) (*route53.DeleteHostedZoneOutput, error)
	ListResourceRecordSets(ctx context.Context, in *route53.ListResourceRecordSetsInput, opts ...func(*route53.Options)) (*route53.ListResourceRecordSetsOutput, error)
	ChangeResourceRecordSets(ctx context.Context, in *route53.ChangeResourceRecordSetsInput, opts ...func(*route53.Options)) (*route53.ChangeResourceRecordSetsOutput, error)
	ListHealthChecks(ctx context.Context, in *route53.ListHealthChecksInput, opts ...func(*route53.Options)) (*route53.ListHealthChecksOutput, error)
	CreateHealthCheck(ctx context.Context, in *route53.CreateHealthCheckInput, opts ...func(*route53.Options)) (*route53.CreateHealthCheckOutput, error)
	DeleteHealthCheck(ctx context.Context, in *route53.DeleteHealthCheckInput, opts ...func(*route53.Options)) (*route53.DeleteHealthCheckOutput, error)
}

// Route53 implements Provider and HealthChecker over the AWS SDK.
type Route53 struct {
	API Route53API
	// HostedZoneID, when set, is used if the credentials cannot list zones.
	HostedZoneID string
}

// NewRoute53 builds a client from a static access key pair. Route53 is
// global, so region only picks the signing endpoint.
func NewRoute53(accessKeyID, secretAccessKey, region, hostedZoneID string) *Route53 {
	if region == "" {
		region = "us-east-1"
	}
	cfg := aws.Config{
		Region:      region,
		Credentials: credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, ""),
	}
	return &Route53{API: route53.NewFromConfig(cfg), HostedZoneID: trimZoneID(hostedZoneID)}
}

// Name implements Provider.
func (r *Route53) Name() string { return ProviderRoute53 }

// Capabilities implements Provider.
func (r *Route53) Capabilities() Capabilities {
	return Capabilities{Routing: true, HealthChecks: true}
}

func trimZoneID(id string) string { return strings.TrimPrefix(id, "/hostedzone/") }

func r53Zone(hz types.HostedZone) Zone {
	z := Zone{ID: trimZoneID(aws.ToString(hz.Id)), Name: NormalizeDomain(unescapeOctal(aws.ToString(hz.Name))), Provider: ProviderRoute53, Status: "active"}
	if hz.ResourceRecordSetCount != nil {
		n := int(*hz.ResourceRecordSetCount)
		z.RecordCount = &n
	}
	if hz.Config != nil && hz.Config.PrivateZone {
		z.Private = true
	}
	return z
}

// ListZones implements Provider (ListHostedZones, then GetHostedZone per zone for name servers).
func (r *Route53) ListZones(ctx context.Context) ([]Zone, error) {
	var zones []Zone
	var marker *string
	for {
		out, err := r.API.ListHostedZones(ctx, &route53.ListHostedZonesInput{Marker: marker})
		if err != nil {
			if r.HostedZoneID != "" {
				z, gerr := r.GetZone(ctx, r.HostedZoneID)
				if gerr == nil {
					return []Zone{z}, nil
				}
			}
			return nil, fmt.Errorf("route53: list hosted zones: %w", err)
		}
		for _, hz := range out.HostedZones {
			zones = append(zones, r53Zone(hz))
		}
		if !out.IsTruncated || out.NextMarker == nil {
			break
		}
		marker = out.NextMarker
	}
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for i := range zones {
		if zones[i].Private {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if full, err := r.GetZone(ctx, zones[i].ID); err == nil {
				zones[i].NameServers = full.NameServers
			}
		}()
	}
	wg.Wait()
	return zones, nil
}

// GetZone implements Provider (GetHostedZone).
func (r *Route53) GetZone(ctx context.Context, id string) (Zone, error) {
	out, err := r.API.GetHostedZone(ctx, &route53.GetHostedZoneInput{Id: aws.String(trimZoneID(id))})
	if err != nil {
		return Zone{}, fmt.Errorf("route53: get hosted zone: %w", err)
	}
	z := r53Zone(*out.HostedZone)
	if out.DelegationSet != nil {
		z.NameServers = normalizeAll(out.DelegationSet.NameServers)
	}
	return z, nil
}

// CreateZone implements Provider (CreateHostedZone, public).
func (r *Route53) CreateZone(ctx context.Context, name string) (Zone, error) {
	ref := fmt.Sprintf("%s-%d", NormalizeDomain(name), time.Now().UnixNano())
	out, err := r.API.CreateHostedZone(ctx, &route53.CreateHostedZoneInput{Name: aws.String(NormalizeDomain(name)), CallerReference: aws.String(ref)})
	if err != nil {
		return Zone{}, fmt.Errorf("route53: create hosted zone: %w", err)
	}
	z := r53Zone(*out.HostedZone)
	if out.DelegationSet != nil {
		z.NameServers = normalizeAll(out.DelegationSet.NameServers)
	}
	return z, nil
}

// DeleteZone implements Provider (DeleteHostedZone). Route53 itself refuses
// while non NS/SOA sets remain, so callers delete those first.
func (r *Route53) DeleteZone(ctx context.Context, id string) error {
	if _, err := r.API.DeleteHostedZone(ctx, &route53.DeleteHostedZoneInput{Id: aws.String(trimZoneID(id))}); err != nil {
		return fmt.Errorf("route53: delete hosted zone: %w", err)
	}
	return nil
}
