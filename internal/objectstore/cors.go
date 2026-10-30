package objectstore

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

// CORSRule is one bucket CORS rule in the shape providers document it.
type CORSRule struct {
	AllowedOrigins []string `json:"allowed_origins"`
	AllowedMethods []string `json:"allowed_methods"`
	AllowedHeaders []string `json:"allowed_headers"`
	ExposeHeaders  []string `json:"expose_headers"`
	MaxAgeSeconds  int32    `json:"max_age_seconds"`
}

// CORS check statuses.
const (
	CORSOK         = "ok"
	CORSMissing    = "missing"
	CORSApplied    = "applied"
	CORSManual     = "manual"
	CORSUnverified = "unverified"
)

const corsMaxAgeSeconds int32 = 3600

// CORSCheck is the CORS step of the asset probe. Rule is the exact rule to add when Status is missing, manual or unverified.
type CORSCheck struct {
	Status  string   `json:"status"`
	Origin  string   `json:"origin"`
	Rule    CORSRule `json:"rule"`
	Message string   `json:"message,omitempty"`
}

// UploadCORSRule is the rule a browser needs to PUT to a presigned URL from origin.
func UploadCORSRule(origin string) CORSRule {
	return CORSRule{
		AllowedOrigins: []string{origin},
		AllowedMethods: []string{"PUT", "GET", "HEAD"},
		AllowedHeaders: []string{"Content-Type"},
		ExposeHeaders:  []string{"ETag"},
		MaxAgeSeconds:  corsMaxAgeSeconds,
	}
}

// CORSCovers reports whether any rule lets origin PUT with a Content-Type header.
func CORSCovers(rules []CORSRule, origin string) bool {
	for _, r := range rules {
		if !slices.ContainsFunc(r.AllowedOrigins, func(o string) bool { return o == "*" || strings.EqualFold(o, origin) }) {
			continue
		}
		if !slices.ContainsFunc(r.AllowedMethods, func(m string) bool { return strings.EqualFold(m, "PUT") }) {
			continue
		}
		if slices.ContainsFunc(r.AllowedHeaders, func(h string) bool { return h == "*" || strings.EqualFold(h, "Content-Type") }) {
			return true
		}
	}
	return false
}

// GetCORS reads the bucket CORS rules; a bucket with none returns an empty list.
func (c *Client) GetCORS(ctx context.Context) ([]CORSRule, error) {
	out, err := c.s3.GetBucketCors(ctx, &s3.GetBucketCorsInput{Bucket: aws.String(c.bucket)})
	if err != nil {
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) && apiErr.ErrorCode() == "NoSuchCORSConfiguration" {
			return nil, nil
		}
		return nil, fmt.Errorf("objectstore: get cors: %w", err)
	}
	rules := make([]CORSRule, 0, len(out.CORSRules))
	for _, r := range out.CORSRules {
		rules = append(rules, CORSRule{
			AllowedOrigins: r.AllowedOrigins, AllowedMethods: r.AllowedMethods, AllowedHeaders: r.AllowedHeaders,
			ExposeHeaders: r.ExposeHeaders, MaxAgeSeconds: aws.ToInt32(r.MaxAgeSeconds),
		})
	}
	return rules, nil
}

// PutCORS replaces the bucket CORS configuration with rules.
func (c *Client) PutCORS(ctx context.Context, rules []CORSRule) error {
	conv := make([]s3types.CORSRule, 0, len(rules))
	for _, r := range rules {
		conv = append(conv, s3types.CORSRule{
			AllowedOrigins: r.AllowedOrigins, AllowedMethods: r.AllowedMethods, AllowedHeaders: r.AllowedHeaders,
			ExposeHeaders: r.ExposeHeaders, MaxAgeSeconds: aws.Int32(r.MaxAgeSeconds),
		})
	}
	_, err := c.s3.PutBucketCors(ctx, &s3.PutBucketCorsInput{
		Bucket: aws.String(c.bucket), CORSConfiguration: &s3types.CORSConfiguration{CORSRules: conv},
	})
	if err != nil {
		return fmt.Errorf("objectstore: put cors: %w", err)
	}
	return nil
}

// CheckCORS verifies browser upload from origin works and, when apply is set, appends the rule if the key may write it.
// Existing rules are kept: the new rule is added, never substituted.
func CheckCORS(ctx context.Context, c *Client, origin string, apply bool) CORSCheck {
	res := CORSCheck{Origin: origin, Rule: UploadCORSRule(origin)}
	existing, err := c.GetCORS(ctx)
	if err != nil {
		reason, msg := Classify(err)
		res.Status = CORSUnverified
		res.Message = "could not read the bucket CORS rules (" + reason + "): " + msg + ". Add the rule below at the provider if browser uploads fail."
		return res
	}
	if CORSCovers(existing, origin) {
		res.Status = CORSOK
		return res
	}
	res.Status = CORSMissing
	res.Message = "no CORS rule lets " + origin + " upload with PUT. Re-run the check with fix_cors to add it, or add the rule below at the provider."
	if apply {
		return applyCORS(ctx, c, res, existing)
	}
	return res
}

func applyCORS(ctx context.Context, c *Client, res CORSCheck, existing []CORSRule) CORSCheck {
	if err := c.PutCORS(ctx, append(slices.Clone(existing), res.Rule)); err != nil {
		_, msg := Classify(err)
		res.Status = CORSManual
		res.Message = "this key cannot change the bucket CORS rules (" + msg + "). Add the rule below at the provider, then re-run the check."
		return res
	}
	res.Status = CORSApplied
	res.Message = "added a CORS rule for " + res.Origin
	return res
}
