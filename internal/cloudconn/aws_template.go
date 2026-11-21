package cloudconn

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
)

// PolicyDocument is an IAM policy document in the shape AWS accepts.
type PolicyDocument struct {
	Version   string            `json:"Version"`
	Statement []PolicyStatement `json:"Statement"`
}

// PolicyStatement is one IAM statement. Condition maps operator to key to values.
type PolicyStatement struct {
	Sid       string                         `json:"Sid"`
	Effect    string                         `json:"Effect"`
	Action    []string                       `json:"Action"`
	Resource  []string                       `json:"Resource"`
	Condition map[string]map[string][]string `json:"Condition,omitempty"`
}

// PolicyOptions shapes a rendered policy. The zero value is filled from DefaultPolicyOptions.
type PolicyOptions struct {
	// Regions becomes an aws:RequestedRegion condition on regional statements; empty means no limit.
	Regions      []string
	TagKey       string
	TagValue     string
	BucketPrefix string
	RoleName     string
	// HostedZoneIDs scopes Route 53 zone statements by ARN; empty falls back to every zone.
	HostedZoneIDs []string
}

// DefaultPolicyOptions reads the tag key and bucket prefix from env so an operator can rename them.
func DefaultPolicyOptions() PolicyOptions {
	get := func(k, def string) string {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
		return def
	}
	return PolicyOptions{
		TagKey: get("APP_CLOUD_AWS_TAG_KEY", "levelrail:managed"), TagValue: "true",
		BucketPrefix: get("APP_CLOUD_AWS_BUCKET_PREFIX", "levelrail-"), RoleName: get("APP_CLOUD_AWS_ROLE_NAME", "LevelrailAccess"),
	}
}

func (o PolicyOptions) filled() PolicyOptions {
	d := DefaultPolicyOptions()
	if o.TagKey == "" {
		o.TagKey = d.TagKey
	}
	if o.TagValue == "" {
		o.TagValue = d.TagValue
	}
	if o.BucketPrefix == "" {
		o.BucketPrefix = d.BucketPrefix
	}
	if o.RoleName == "" {
		o.RoleName = d.RoleName
	}
	return o
}

func titleCase(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// RenderPolicy builds the permission policy for the named capabilities from the registry.
func RenderPolicy(names []string, opt PolicyOptions) (PolicyDocument, error) {
	caps, err := ResolveCapabilities(names)
	if err != nil {
		return PolicyDocument{}, err
	}
	return renderCaps(caps, opt.filled()), nil
}

func renderCaps(caps []Capability, opt PolicyOptions) PolicyDocument {
	rep := strings.NewReplacer("{bucket_prefix}", opt.BucketPrefix, "{self_role}", "arn:*:iam::*:role/"+opt.RoleName)
	doc := PolicyDocument{Version: "2012-10-17", Statement: []PolicyStatement{}}
	for _, c := range caps {
		for i, s := range c.Statements {
			st := PolicyStatement{Sid: fmt.Sprintf("%s%d", titleCase(c.Name), i+1), Effect: "Allow", Action: append([]string(nil), s.Actions...)}
			for _, r := range s.Resources {
				if r == "{hosted_zone_ids}" {
					st.Resource = append(st.Resource, hostedZoneARNs(opt.HostedZoneIDs)...)
					continue
				}
				st.Resource = append(st.Resource, rep.Replace(r))
			}
			cond := map[string]map[string][]string{}
			add := func(op, key string, vals []string) {
				if cond[op] == nil {
					cond[op] = map[string][]string{}
				}
				cond[op][key] = vals
			}
			if !s.Global && len(opt.Regions) > 0 {
				add("StringEquals", "aws:RequestedRegion", sortedCopy(opt.Regions))
			}
			switch s.Tag {
			case TagResource:
				add("StringEquals", "aws:ResourceTag/"+opt.TagKey, []string{opt.TagValue})
			case TagRequest:
				add("StringEquals", "aws:RequestTag/"+opt.TagKey, []string{opt.TagValue})
			}
			for _, x := range s.Conditions {
				add(x.Operator, x.Key, x.Values)
			}
			if len(cond) > 0 {
				st.Condition = cond
			}
			doc.Statement = append(doc.Statement, st)
		}
	}
	return doc
}

func hostedZoneARNs(ids []string) []string {
	if len(ids) == 0 {
		return []string{"arn:*:route53:::hostedzone/*"}
	}
	out := make([]string, 0, len(ids))
	for _, id := range sortedCopy(ids) {
		out = append(out, "arn:*:route53:::hostedzone/"+id)
	}
	return out
}

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

// PolicyJSON renders the plain JSON policy for the named capabilities.
func PolicyJSON(names []string, opt PolicyOptions) (string, error) {
	doc, err := RenderPolicy(names, opt)
	if err != nil {
		return "", err
	}
	return marshalIndent(doc)
}

// BoundaryPolicy is the permission ceiling: every capability, plus a deny on identity changes.
func BoundaryPolicy(opt PolicyOptions) PolicyDocument {
	opt = opt.filled()
	caps, _ := ResolveCapabilities(nil)
	doc := renderCaps(caps, opt)
	doc = collapseManaged(doc, opt)
	doc.Statement = append(doc.Statement, PolicyStatement{
		Sid: "DenyIdentityChanges", Effect: "Deny", Action: append([]string(nil), awsBoundaryDeny...), Resource: []string{"*"},
	})
	return doc
}

func marshalIndent(v any) (string, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return "", err
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

var (
	externalIDRe = regexp.MustCompile(`^[A-Za-z0-9+=,.@:/_-]{2,1000}$`)
	stackNameRe  = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9-]{0,127}$`)
	hostedZoneRe = regexp.MustCompile(`^[A-Z0-9]{1,32}$`)
	s3HostRe     = regexp.MustCompile(`^([a-z0-9][a-z0-9.-]{1,61}\.)?s3[.-]([a-z0-9-]+\.)?amazonaws\.com(\.cn)?$`)
	roleNameRe   = regexp.MustCompile(`^[A-Za-z0-9+=,.@_-]{1,64}$`)
	principalRe  = regexp.MustCompile(`^arn:aws[a-z-]*:iam::[0-9]{12}:(root|role/[A-Za-z0-9+=,.@_/-]+|user/[A-Za-z0-9+=,.@_/-]+)$`)
)

// TemplateInput is everything the generators need about one account link.
type TemplateInput struct {
	RoleName          string
	TrustPrincipalARN string
	ExternalID        string
	Capabilities      []string
	Regions           []string
	BucketPrefix      string
	HostedZoneIDs     []string
}

func (in TemplateInput) validate() (PolicyOptions, error) {
	opt := PolicyOptions{Regions: in.Regions, BucketPrefix: in.BucketPrefix, RoleName: in.RoleName, HostedZoneIDs: in.HostedZoneIDs}.filled()
	if !roleNameRe.MatchString(opt.RoleName) {
		return opt, invalid("role name must be 1 to 64 IAM name characters")
	}
	if !principalRe.MatchString(in.TrustPrincipalARN) {
		return opt, invalid("trusted principal must be an IAM account, role or user ARN")
	}
	if !externalIDRe.MatchString(in.ExternalID) {
		return opt, invalid("external id must be 2 to 1000 characters from [A-Za-z0-9+=,.@:/_-]")
	}
	for _, z := range in.HostedZoneIDs {
		if !hostedZoneRe.MatchString(z) {
			return opt, invalid("hosted zone id %q is not valid", z)
		}
	}
	for _, r := range in.Regions {
		if !awsRegionRe.MatchString(r) {
			return opt, invalid("region %q is not an AWS region", r)
		}
	}
	return opt, nil
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// indent prefixes every line of s with n spaces.
func indent(s string, n int) string {
	pad := strings.Repeat(" ", n)
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = pad + l
	}
	return strings.Join(lines, "\n")
}

// CloudFormationTemplate returns the YAML that creates the cross-account role: ExternalId trust,
// the chosen capability tiers, tag and region conditions, and a permission boundary.
func CloudFormationTemplate(in TemplateInput) (string, error) {
	opt, err := in.validate()
	if err != nil {
		return "", err
	}
	role, err := RenderPolicy(in.Capabilities, opt)
	if err != nil {
		return "", err
	}
	roleJSON, err := marshalIndent(role)
	if err != nil {
		return "", err
	}
	boundJSON, err := marshalIndent(BoundaryPolicy(opt))
	if err != nil {
		return "", err
	}
	caps, _ := ResolveCapabilities(in.Capabilities)
	var names []string
	for _, c := range caps {
		if !c.Always {
			names = append(names, c.Name)
		}
	}
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format+"\n", a...) }
	w("AWSTemplateFormatVersion: \"2010-09-09\"")
	w("Description: %s", jsonString("Levelrail access role ("+strings.Join(names, ", ")+")"))
	w("Parameters:")
	w("  TrustedPrincipalArn:")
	w("    Type: String")
	w("    Default: %s", jsonString(in.TrustPrincipalARN))
	w("    Description: The Levelrail identity allowed to assume the role.")
	w("  ExternalId:")
	w("    Type: String")
	w("    Default: %s", jsonString(in.ExternalID))
	w("    Description: Per account value Levelrail must present when assuming the role.")
	w("Resources:")
	w("  BoundaryPolicy:")
	w("    Type: AWS::IAM::ManagedPolicy")
	w("    Properties:")
	w("      ManagedPolicyName: %s", jsonString(opt.RoleName+"-boundary"))
	w("      PolicyDocument:")
	w("%s", indent(boundJSON, 8))
	w("  AccessRole:")
	w("    Type: AWS::IAM::Role")
	w("    Properties:")
	w("      RoleName: %s", jsonString(opt.RoleName))
	w("      MaxSessionDuration: 3600")
	w("      PermissionsBoundary: !Ref BoundaryPolicy")
	w("      AssumeRolePolicyDocument:")
	w("        Version: \"2012-10-17\"")
	w("        Statement:")
	w("          - Effect: Allow")
	w("            Principal:")
	w("              AWS: !Ref TrustedPrincipalArn")
	w("            Action: sts:AssumeRole")
	w("            Condition:")
	w("              StringEquals:")
	w("                sts:ExternalId: !Ref ExternalId")
	w("              StringLike:")
	w("                sts:RoleSessionName: \"levelrail*\"")
	w("      Policies:")
	w("        - PolicyName: levelrail-access")
	w("          PolicyDocument:")
	w("%s", indent(roleJSON, 12))
	w("      Tags:")
	w("        - Key: %s", jsonString(opt.TagKey))
	w("          Value: %s", jsonString(opt.TagValue))
	w("Outputs:")
	w("  RoleArn:")
	w("    Description: Paste this into Levelrail.")
	w("    Value: !GetAtt AccessRole.Arn")
	return b.String(), nil
}

// consoleHost returns the CloudFormation console host for the partition of region.
func consoleHost(region string) string {
	switch {
	case strings.HasPrefix(region, "cn-"):
		return region + ".console.amazonaws.cn"
	case strings.HasPrefix(region, "us-gov-"):
		return region + ".console.amazonaws-us-gov.com"
	}
	return region + ".console.aws.amazon.com"
}

// QuickCreateURL builds the CloudFormation quick-create link. The template must already be
// hosted as an S3 object, the only source CloudFormation accepts for templateURL.
func QuickCreateURL(in TemplateInput, templateURL, stackName, region string) (string, error) {
	if _, err := in.validate(); err != nil {
		return "", err
	}
	if !awsRegionRe.MatchString(region) {
		return "", invalid("region must be an AWS region")
	}
	if u, err := url.Parse(templateURL); err != nil || u.Scheme != "https" || !s3HostRe.MatchString(u.Host) || len(u.Path) < 2 {
		return "", invalid("template url must be an https S3 object url")
	}
	if !stackNameRe.MatchString(stackName) {
		return "", invalid("stack name must start with a letter and use letters, digits and hyphens, up to 128 characters")
	}
	q := url.Values{}
	q.Set("templateURL", templateURL)
	q.Set("stackName", stackName)
	q.Set("param_TrustedPrincipalArn", in.TrustPrincipalARN)
	q.Set("param_ExternalId", in.ExternalID)
	return fmt.Sprintf("https://%s/cloudformation/home?region=%s#/stacks/create/review?%s", consoleHost(region), url.QueryEscape(region), q.Encode()), nil
}

// TerraformSnippet returns the same role, boundary and policy as HCL.
func TerraformSnippet(in TemplateInput) (string, error) {
	opt, err := in.validate()
	if err != nil {
		return "", err
	}
	role, err := RenderPolicy(in.Capabilities, opt)
	if err != nil {
		return "", err
	}
	roleJSON, err := marshalIndent(role)
	if err != nil {
		return "", err
	}
	boundJSON, err := marshalIndent(BoundaryPolicy(opt))
	if err != nil {
		return "", err
	}
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format+"\n", a...) }
	w("variable \"trusted_principal_arn\" {\n  type    = string\n  default = %s\n}\n", jsonString(in.TrustPrincipalARN))
	w("variable \"external_id\" {\n  type    = string\n  default = %s\n}\n", jsonString(in.ExternalID))
	w("resource \"aws_iam_policy\" \"levelrail_boundary\" {\n  name   = %s\n  policy = <<-POLICY\n%s\n  POLICY\n}\n", jsonString(opt.RoleName+"-boundary"), indent(boundJSON, 4))
	w("resource \"aws_iam_role\" \"levelrail\" {")
	w("  name                 = %s", jsonString(opt.RoleName))
	w("  max_session_duration = 3600")
	w("  permissions_boundary = aws_iam_policy.levelrail_boundary.arn")
	w("  tags                 = { %s = %s }", jsonString(opt.TagKey), jsonString(opt.TagValue))
	w("  assume_role_policy = jsonencode({")
	w("    Version = \"2012-10-17\"")
	w("    Statement = [{")
	w("      Effect    = \"Allow\"")
	w("      Principal = { AWS = var.trusted_principal_arn }")
	w("      Action    = \"sts:AssumeRole\"")
	w("      Condition = {")
	w("        StringEquals = { \"sts:ExternalId\" = var.external_id }")
	w("        StringLike   = { \"sts:RoleSessionName\" = \"levelrail*\" }")
	w("      }")
	w("    }]")
	w("  })")
	w("}\n")
	w("resource \"aws_iam_role_policy\" \"levelrail\" {\n  name   = \"levelrail-access\"\n  role   = aws_iam_role.levelrail.id\n  policy = <<-POLICY\n%s\n  POLICY\n}\n", indent(roleJSON, 4))
	w("output \"role_arn\" {\n  value = aws_iam_role.levelrail.arn\n}")
	return b.String(), nil
}

// boundaryWildcardServices are collapsed to service wildcards in the permission boundary so it
// fits the managed policy size limit. The role policy keeps the exact actions and resources.
var boundaryWildcardServices = map[string]bool{
	"rds": true, "elasticache": true, "ecr": true, "acm": true, "cloudfront": true,
	"secretsmanager": true, "ssm": true, "cloudtrail": true, "cloudwatch": true, "logs": true,
}

func collapseManaged(doc PolicyDocument, opt PolicyOptions) PolicyDocument {
	kept := doc.Statement[:0:0]
	seen := map[string]bool{}
	for _, st := range doc.Statement {
		var rest []string
		for _, a := range st.Action {
			svc, _, _ := strings.Cut(a, ":")
			if boundaryWildcardServices[svc] {
				seen[svc] = true
				continue
			}
			rest = append(rest, a)
		}
		if len(rest) == 0 {
			continue
		}
		st.Action = rest
		kept = append(kept, st)
	}
	regional, global := []string{}, []string{}
	svcs := make([]string, 0, len(seen))
	for k := range seen {
		svcs = append(svcs, k)
	}
	sort.Strings(svcs)
	for _, svc := range svcs {
		if svc == "cloudfront" {
			global = append(global, svc+":*")
		} else {
			regional = append(regional, svc+":*")
		}
	}
	if len(regional) > 0 {
		st := PolicyStatement{Sid: "ManagedServices", Effect: "Allow", Action: regional, Resource: []string{"*"}}
		if len(opt.Regions) > 0 {
			st.Condition = map[string]map[string][]string{"StringEquals": {"aws:RequestedRegion": sortedCopy(opt.Regions)}}
		}
		kept = append(kept, st)
	}
	if len(global) > 0 {
		kept = append(kept, PolicyStatement{Sid: "ManagedGlobalServices", Effect: "Allow", Action: global, Resource: []string{"*"}})
	}
	doc.Statement = kept
	return doc
}
