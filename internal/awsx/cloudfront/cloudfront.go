// Package cloudfront reads distributions and creates cache invalidations over signed REST/XML.
package cloudfront

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/awsx"
)

// Service is global: one host, signed for us-east-1.
var Service = awsx.Service{Name: "cloudfront", Prefix: "cloudfront", SigningName: "cloudfront"}

// ServiceName is the key used in the guard ServicesEnabled map.
const ServiceName = "cloudfront"

const (
	host    = "https://cloudfront.amazonaws.com"
	region  = "us-east-1"
	apiBase = "/2020-05-31/distribution"
	nsDoc   = "http://cloudfront.amazonaws.com/doc/2020-05-31/"
)

// ErrInvalidPaths marks a rejected invalidation path list.
var ErrInvalidPaths = errors.New("cloudfront: invalid paths")

// Client is the lazy CloudFront module.
type Client struct {
	aws *awsx.Client
	// host overrides the endpoint in tests.
	host string
}

// New wraps a shared client.
func New(c *awsx.Client) *Client { return &Client{aws: c, host: host} }

// Distribution is the read view of one distribution.
type Distribution struct {
	ID, ARN, Status, Domain, Comment string
	Enabled                          bool
	Aliases                          []string
}

type distXML struct {
	ID      string   `xml:"Id"`
	ARN     string   `xml:"ARN"`
	Status  string   `xml:"Status"`
	Domain  string   `xml:"DomainName"`
	Comment string   `xml:"Comment"`
	Enabled bool     `xml:"Enabled"`
	Aliases []string `xml:"Aliases>Items>CNAME"`
	Config  struct {
		Comment string   `xml:"Comment"`
		Enabled bool     `xml:"Enabled"`
		Aliases []string `xml:"Aliases>Items>CNAME"`
	} `xml:"DistributionConfig"`
}

func (d distXML) view() Distribution {
	v := Distribution{ID: d.ID, ARN: d.ARN, Status: d.Status, Domain: d.Domain, Comment: d.Comment, Enabled: d.Enabled, Aliases: d.Aliases}
	if v.Comment == "" {
		v.Comment = d.Config.Comment
	}
	if len(v.Aliases) == 0 {
		v.Aliases = d.Config.Aliases
	}
	v.Enabled = v.Enabled || d.Config.Enabled
	return v
}

// ListPage is one page of distributions with the marker for the next.
type ListPage struct {
	Items []Distribution
	Next  string
}

// ListDistributions returns one page; pass the previous Next as marker.
func (c *Client) ListDistributions(ctx context.Context, marker string) (ListPage, error) {
	path := apiBase
	if marker != "" {
		path += "?Marker=" + url.QueryEscape(marker)
	}
	var out struct {
		Next  string    `xml:"NextMarker"`
		Items []distXML `xml:"Items>DistributionSummary"`
	}
	if err := c.aws.RESTGet(ctx, Service, c.host, region, path, &out); err != nil {
		return ListPage{}, err
	}
	page := ListPage{Next: out.Next}
	for _, d := range out.Items {
		page.Items = append(page.Items, d.view())
	}
	return page, nil
}

// GetDistribution reads one distribution by id.
func (c *Client) GetDistribution(ctx context.Context, id string) (Distribution, error) {
	var out distXML
	if err := c.aws.RESTGet(ctx, Service, c.host, region, apiBase+"/"+url.PathEscape(id), &out); err != nil {
		return Distribution{}, err
	}
	return out.view(), nil
}

// Invalidation is one cache invalidation.
type Invalidation struct {
	ID, Status string
	CreatedAt  time.Time
}

type invXML struct {
	ID      string `xml:"Id"`
	Status  string `xml:"Status"`
	Created string `xml:"CreateTime"`
}

func (i invXML) view() Invalidation {
	t, _ := time.Parse(time.RFC3339, i.Created)
	return Invalidation{ID: i.ID, Status: i.Status, CreatedAt: t}
}

// CleanPaths validates and dedupes paths. maxPaths comes from a runtime setting.
func CleanPaths(paths []string, maxPaths int) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" || p[0] != '/' || len(p) > 4000 || strings.ContainsAny(p, "\r\n\x00") {
			return nil, fmt.Errorf("%w: each path must start with /", ErrInvalidPaths)
		}
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: at least one path is required", ErrInvalidPaths)
	}
	if maxPaths > 0 && len(out) > maxPaths {
		return nil, fmt.Errorf("%w: at most %d paths per invalidation", ErrInvalidPaths, maxPaths)
	}
	return out, nil
}

// CreateInvalidation invalidates the given paths. Paths must already be cleaned.
func (c *Client) CreateInvalidation(ctx context.Context, id string, paths []string) (Invalidation, error) {
	ref := make([]byte, 12)
	if _, err := rand.Read(ref); err != nil {
		return Invalidation{}, err
	}
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><InvalidationBatch xmlns="` + nsDoc + `"><Paths><Quantity>`)
	fmt.Fprintf(&b, "%d</Quantity><Items>", len(paths))
	for _, p := range paths {
		b.WriteString("<Path>")
		if err := xml.EscapeText(&b, []byte(p)); err != nil {
			return Invalidation{}, err
		}
		b.WriteString("</Path>")
	}
	b.WriteString("</Items></Paths><CallerReference>lr-" + hex.EncodeToString(ref) + "</CallerReference></InvalidationBatch>")
	var out invXML
	if err := c.aws.REST(ctx, Service, c.host, region, "POST", apiBase+"/"+url.PathEscape(id)+"/invalidation", b.Bytes(), &out); err != nil {
		return Invalidation{}, err
	}
	return out.view(), nil
}

// ListInvalidations returns one page of recent invalidations for a distribution.
func (c *Client) ListInvalidations(ctx context.Context, id, marker string) ([]Invalidation, string, error) {
	path := apiBase + "/" + url.PathEscape(id) + "/invalidation"
	if marker != "" {
		path += "?Marker=" + url.QueryEscape(marker)
	}
	var out struct {
		Next  string   `xml:"NextMarker"`
		Items []invXML `xml:"Items>InvalidationSummary"`
	}
	if err := c.aws.RESTGet(ctx, Service, c.host, region, path, &out); err != nil {
		return nil, "", err
	}
	res := make([]Invalidation, 0, len(out.Items))
	for _, i := range out.Items {
		res = append(res, i.view())
	}
	return res, out.Next, nil
}
