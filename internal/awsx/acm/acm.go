// Package acm requests and reads ACM certificates with DNS validation.
package acm

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/awsx"
	"github.com/GLINCKER/levelrail/internal/cloudguard"
)

// Service is the ACM API (target prefix checked against the botocore model).
var Service = awsx.Service{Name: "acm", Prefix: "acm", SigningName: "acm", TargetPrefix: "CertificateManager"}

// ServiceName is the key used in the guard ServicesEnabled map.
const ServiceName = "acm"

// ErrRefused wraps a guard refusal.
var ErrRefused = errors.New("acm: create refused")

// Guard is the create gate and tag source.
type Guard interface {
	Check(ctx context.Context, r cloudguard.CreateRequest) cloudguard.Decision
	Tags(m cloudguard.TagMeta) map[string]string
}

// Client is the lazy ACM module.
type Client struct {
	aws   *awsx.Client
	guard Guard
}

// New wraps a shared client and guard.
func New(c *awsx.Client, g Guard) *Client { return &Client{aws: c, guard: g} }

// RequestInput describes one public certificate request.
type RequestInput struct {
	Account, Region, Domain string
	AltNames                []string
	// Token makes the request idempotent for a short window; letters and digits only.
	Token string
	Meta  cloudguard.TagMeta
}

// RequestCertificate asks for a public certificate validated by DNS. Public
// ACM certificates are free, so the estimate is zero.
func (c *Client) RequestCertificate(ctx context.Context, in RequestInput) (string, error) {
	tags := c.guard.Tags(in.Meta)
	d := c.guard.Check(ctx, cloudguard.CreateRequest{Account: in.Account, Service: ServiceName, Resource: in.Domain, Tags: tags})
	if !d.Allowed {
		return "", fmt.Errorf("%w: %s", ErrRefused, d.Message)
	}
	req := map[string]any{"DomainName": in.Domain, "ValidationMethod": "DNS", "Tags": awsx.TagList(tags)}
	if len(in.AltNames) > 0 {
		req["SubjectAlternativeNames"] = in.AltNames
	}
	if in.Token != "" {
		req["IdempotencyToken"] = in.Token
	}
	var out struct {
		ARN string `json:"CertificateArn"`
	}
	if err := c.aws.JSON(ctx, Service, in.Region, "RequestCertificate", req, &out); err != nil {
		return "", err
	}
	return out.ARN, nil
}

// ValidationRecord is the CNAME to publish so ACM can validate a domain.
type ValidationRecord struct {
	Domain, Name, Type, Value, Status string
}

// Certificate is the read view of one certificate.
type Certificate struct {
	ARN, Domain, Status string
	NotAfter            time.Time
	Records             []ValidationRecord
}

// DescribeCertificate returns the status and the DNS validation records. The
// records appear a few seconds after the request, so callers may need to poll.
func (c *Client) DescribeCertificate(ctx context.Context, region, arn string) (Certificate, error) {
	var out struct {
		Cert struct {
			ARN      string         `json:"CertificateArn"`
			Domain   string         `json:"DomainName"`
			Status   string         `json:"Status"`
			NotAfter awsx.EpochTime `json:"NotAfter"`
			Options  []struct {
				Domain string `json:"DomainName"`
				Status string `json:"ValidationStatus"`
				Record *struct {
					Name  string `json:"Name"`
					Type  string `json:"Type"`
					Value string `json:"Value"`
				} `json:"ResourceRecord"`
			} `json:"DomainValidationOptions"`
		} `json:"Certificate"`
	}
	if err := c.aws.JSON(ctx, Service, region, "DescribeCertificate", map[string]string{"CertificateArn": arn}, &out); err != nil {
		return Certificate{}, err
	}
	cert := Certificate{ARN: out.Cert.ARN, Domain: out.Cert.Domain, Status: out.Cert.Status, NotAfter: out.Cert.NotAfter.Time}
	for _, o := range out.Cert.Options {
		if o.Record == nil {
			continue
		}
		cert.Records = append(cert.Records, ValidationRecord{Domain: o.Domain, Name: o.Record.Name, Type: o.Record.Type, Value: o.Record.Value, Status: o.Status})
	}
	return cert, nil
}

// Summary is one row of ListCertificates.
type Summary struct {
	ARN, Domain, Status string
}

// ListCertificates pages through every certificate in the region.
func (c *Client) ListCertificates(ctx context.Context, region string) ([]Summary, error) {
	var res []Summary
	token := ""
	for {
		in := map[string]any{}
		if token != "" {
			in["NextToken"] = token
		}
		var out struct {
			List []struct {
				ARN    string `json:"CertificateArn"`
				Domain string `json:"DomainName"`
				Status string `json:"Status"`
			} `json:"CertificateSummaryList"`
			Next string `json:"NextToken"`
		}
		if err := c.aws.JSON(ctx, Service, region, "ListCertificates", in, &out); err != nil {
			return nil, err
		}
		for _, s := range out.List {
			res = append(res, Summary{ARN: s.ARN, Domain: s.Domain, Status: s.Status})
		}
		if out.Next == "" {
			return res, nil
		}
		token = out.Next
	}
}
