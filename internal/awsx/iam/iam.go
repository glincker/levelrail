// Package iam reads IAM role policy attachments over the signed IAM Query API (version 2010-05-08).
package iam

import (
	"context"
	"net/url"
	"strings"

	"github.com/glincker/levelrail/internal/awsx"
)

// Service is the IAM Query API; IAM is global and signs in us-east-1.
var Service = awsx.Service{Name: "iam", Prefix: "iam", SigningName: "iam"}

const (
	version  = "2010-05-08"
	maxPages = 10
	// Region is the signing region for the global IAM endpoint.
	Region = "us-east-1"
)

// Client is the lazy IAM module; it makes no call until a method runs.
type Client struct{ aws *awsx.Client }

// New wraps a shared client.
func New(c *awsx.Client) *Client { return &Client{aws: c} }

// RoleName extracts the role name from a role ARN, or empty.
func RoleName(arn string) string {
	_, name, ok := strings.Cut(arn, ":role/")
	if !ok {
		return ""
	}
	return name[strings.LastIndex(name, "/")+1:]
}

// ListAttachedRolePolicies returns the ARNs of managed policies attached to a role.
func (c *Client) ListAttachedRolePolicies(ctx context.Context, roleName string) ([]string, error) {
	var arns []string
	marker := ""
	for i := 0; i < maxPages; i++ {
		v := url.Values{"RoleName": {roleName}}
		if marker != "" {
			v.Set("Marker", marker)
		}
		var out struct {
			Result struct {
				Truncated bool   `xml:"IsTruncated"`
				Marker    string `xml:"Marker"`
				Policies  []struct {
					ARN string `xml:"PolicyArn"`
				} `xml:"AttachedPolicies>member"`
			} `xml:"ListAttachedRolePoliciesResult"`
		}
		if err := c.aws.Query(ctx, Service, Region, "ListAttachedRolePolicies", version, v, &out); err != nil {
			return nil, err
		}
		for _, p := range out.Result.Policies {
			arns = append(arns, p.ARN)
		}
		if !out.Result.Truncated || out.Result.Marker == "" {
			break
		}
		marker = out.Result.Marker
	}
	return arns, nil
}
