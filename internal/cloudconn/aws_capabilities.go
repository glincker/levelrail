package cloudconn

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// Capability names a user can grant to a linked AWS account.
const (
	CapInventory   = "inventory"
	CapDeploy      = "deploy"
	CapCost        = "cost"
	CapS3          = "s3"
	CapDNS         = "dns"
	CapDestructive = "destructive"
	capVerify      = "verify"
)

// TagMode says which tag condition guards a statement.
type TagMode string

const (
	// TagNone has no tag condition: reads, and actions that cannot be tag scoped.
	TagNone TagMode = ""
	// TagResource requires the target resource to carry the managed tag.
	TagResource TagMode = "resource"
	// TagRequest requires the create request to set the managed tag.
	TagRequest TagMode = "request"
)

// CapCondition is one extra IAM condition operator, key and values.
type CapCondition struct {
	Operator string
	Key      string
	Values   []string
}

// CapStatement is one allow statement of a capability. Resources may use the {bucket_prefix}
// and {self_role} tokens; Global statements skip the region condition (IAM, S3, Route 53, Cost Explorer).
type CapStatement struct {
	Actions    []string
	Resources  []string
	Tag        TagMode
	Global     bool
	Conditions []CapCondition
}

// Capability is one grantable tier. DryRun names an EC2 DryRun probe usable when
// iam:SimulatePrincipalPolicy is not allowed.
type Capability struct {
	Name       string
	Summary    string
	Mutating   bool
	Always     bool
	Hint       string
	DryRun     string
	Statements []CapStatement
}

const all = "*"

// awsCapabilities is the only place AWS permission lists live. When AWS renames or splits an
// action, edit this table and every template, policy and probe follows.
var awsCapabilities = []Capability{
	{
		Name: capVerify, Always: true, Summary: "Let Levelrail check its own permissions",
		Statements: []CapStatement{{
			Actions: []string{"iam:SimulatePrincipalPolicy"}, Resources: []string{"{self_role}"}, Global: true,
		}},
	},
	{
		Name: CapInventory, Summary: "List instances, networks, volumes and regions", DryRun: "DescribeInstances",
		Hint: "Add the inventory tier to the stack and update it.",
		Statements: []CapStatement{
			{Actions: []string{
				"ec2:DescribeInstances", "ec2:DescribeInstanceTypes", "ec2:DescribeImages", "ec2:DescribeVpcs", "ec2:DescribeSubnets",
				"ec2:DescribeSecurityGroups", "ec2:DescribeVolumes", "ec2:DescribeAddresses", "ec2:DescribeKeyPairs", "ec2:DescribeAvailabilityZones",
			}, Resources: []string{all}},
			{Actions: []string{"ec2:DescribeRegions", "tag:GetResources"}, Resources: []string{all}, Global: true},
		},
	},
	{
		Name: CapDeploy, Mutating: true, Summary: "Create and manage tagged instances, volumes and security groups", DryRun: "CreateSecurityGroup",
		Hint: "Add the deploy tier to the stack and update it.",
		Statements: []CapStatement{
			{Actions: []string{"ec2:RunInstances", "ec2:CreateVolume", "ec2:CreateSecurityGroup", "ec2:CreateKeyPair", "ec2:AllocateAddress"},
				Resources: []string{"arn:*:ec2:*:*:instance/*", "arn:*:ec2:*:*:volume/*", "arn:*:ec2:*:*:security-group/*", "arn:*:ec2:*:*:key-pair/*", "arn:*:ec2:*:*:elastic-ip/*"},
				Tag:       TagRequest},
			{Actions: []string{"ec2:RunInstances"}, Resources: []string{
				"arn:*:ec2:*::image/*", "arn:*:ec2:*:*:subnet/*", "arn:*:ec2:*:*:network-interface/*", "arn:*:ec2:*:*:security-group/*", "arn:*:ec2:*:*:key-pair/*",
			}},
			{Actions: []string{
				"ec2:StartInstances", "ec2:StopInstances", "ec2:RebootInstances", "ec2:ModifyInstanceAttribute", "ec2:AttachVolume", "ec2:AssociateAddress",
				"ec2:AuthorizeSecurityGroupIngress", "ec2:AuthorizeSecurityGroupEgress", "ec2:RevokeSecurityGroupIngress", "ec2:RevokeSecurityGroupEgress",
			}, Resources: []string{all}, Tag: TagResource},
			{Actions: []string{"ec2:CreateTags"}, Resources: []string{all}, Conditions: []CapCondition{{
				Operator: "StringEquals", Key: "ec2:CreateAction",
				Values: []string{"RunInstances", "CreateVolume", "CreateSecurityGroup", "CreateKeyPair", "AllocateAddress"},
			}}},
		},
	},
	{
		Name: CapCost, Summary: "Read cost and usage", Hint: "Add the cost tier and enable Cost Explorer in the account.",
		Statements: []CapStatement{{
			Actions:   []string{"ce:GetCostAndUsage", "ce:GetCostForecast", "ce:GetDimensionValues", "ce:GetTags", "budgets:ViewBudget"},
			Resources: []string{all}, Global: true,
		}},
	},
	{
		Name: CapS3, Mutating: true, Summary: "Create and use tagged buckets", Hint: "Add the s3 tier to the stack and update it.",
		Statements: []CapStatement{
			{Actions: []string{"s3:ListAllMyBuckets"}, Resources: []string{all}, Global: true},
			{Actions: []string{"s3:CreateBucket", "s3:PutBucketTagging"}, Resources: []string{"arn:*:s3:::{bucket_prefix}*"}, Tag: TagRequest, Global: true},
			{Actions: []string{"s3:ListBucket", "s3:GetBucketLocation", "s3:GetBucketTagging"}, Resources: []string{"arn:*:s3:::*"}, Tag: TagResource, Global: true},
			{Actions: []string{"s3:GetObject", "s3:PutObject", "s3:AbortMultipartUpload", "s3:ListMultipartUploadParts"}, Resources: []string{"arn:*:s3:::{bucket_prefix}*/*"}, Global: true},
		},
	},
	{
		Name: CapDNS, Mutating: true, Summary: "Read zones and change records in tagged zones", Hint: "Add the dns tier to the stack and update it.",
		Statements: []CapStatement{
			{Actions: []string{"route53:ListHostedZones", "route53:ListHostedZonesByName", "route53:GetChange"}, Resources: []string{all}, Global: true},
			{Actions: []string{"route53:GetHostedZone", "route53:ListResourceRecordSets", "route53:ChangeResourceRecordSets"}, Resources: []string{"arn:*:route53:::hostedzone/*"}, Tag: TagResource, Global: true},
		},
	},
	{
		Name: CapDestructive, Mutating: true, Summary: "Delete tagged resources", Hint: "Add the destructive tier only when Levelrail should delete resources.",
		Statements: []CapStatement{
			{Actions: []string{
				"ec2:TerminateInstances", "ec2:DeleteVolume", "ec2:DeleteSecurityGroup", "ec2:DeleteKeyPair", "ec2:ReleaseAddress", "ec2:DetachVolume",
			}, Resources: []string{all}, Tag: TagResource},
			{Actions: []string{"s3:DeleteBucket"}, Resources: []string{"arn:*:s3:::*"}, Tag: TagResource, Global: true},
			{Actions: []string{"s3:DeleteObject"}, Resources: []string{"arn:*:s3:::{bucket_prefix}*/*"}, Global: true},
			{Actions: []string{"route53:DeleteHostedZone"}, Resources: []string{"arn:*:route53:::hostedzone/*"}, Tag: TagResource, Global: true},
		},
	},
}

// awsBoundaryDeny is denied in the permission boundary so the role can never change identities,
// whatever a tier grants. iam:SimulatePrincipalPolicy stays allowed.
var awsBoundaryDeny = []string{
	"iam:Create*", "iam:Delete*", "iam:Put*", "iam:Attach*", "iam:Detach*", "iam:Update*", "iam:Add*", "iam:Remove*",
	"iam:Set*", "iam:Tag*", "iam:Untag*", "iam:Upload*", "iam:Enable*", "iam:Disable*", "organizations:*", "account:*",
}

// AWSCapabilities returns the registry, always-on entries excluded.
func AWSCapabilities() []Capability {
	out := make([]Capability, 0, len(awsCapabilities))
	for _, c := range awsCapabilities {
		if !c.Always {
			out = append(out, c)
		}
	}
	return out
}

// ResolveCapabilities validates names against the registry and returns them in registry order,
// always-on entries first. An empty list selects every tier.
func ResolveCapabilities(names []string) ([]Capability, error) {
	want := map[string]bool{}
	for _, n := range names {
		n = strings.ToLower(strings.TrimSpace(n))
		if n == capVerify {
			continue
		}
		if capByName(n) == nil {
			return nil, invalid("unknown capability %q", n)
		}
		want[n] = true
	}
	out := make([]Capability, 0, len(awsCapabilities))
	for _, c := range awsCapabilities {
		if c.Always || len(names) == 0 || want[c.Name] {
			out = append(out, c)
		}
	}
	return out, nil
}

func capByName(n string) *Capability {
	for i := range awsCapabilities {
		if awsCapabilities[i].Name == n && !awsCapabilities[i].Always {
			return &awsCapabilities[i]
		}
	}
	return nil
}

// ActionList returns every IAM action of the capability, sorted and unique.
func (c Capability) ActionList() []string {
	seen := map[string]bool{}
	for _, s := range c.Statements {
		for _, a := range s.Actions {
			seen[a] = true
		}
	}
	out := make([]string, 0, len(seen))
	for a := range seen {
		out = append(out, a)
	}
	sort.Strings(out)
	return out
}

// CapabilityResult is one row of the verify connection report.
type CapabilityResult struct {
	Name     string   `json:"name"`
	OK       bool     `json:"ok"`
	Verified bool     `json:"verified"`
	Method   string   `json:"method"`
	Missing  []string `json:"missing,omitempty"`
	Hint     string   `json:"hint,omitempty"`
}

// Probe methods.
const (
	ProbeSimulate = "simulate"
	ProbeDryRun   = "dry_run"
	ProbeNone     = "none"
)

// ProbeCapabilities checks which capabilities the credential really has. It prefers
// iam:SimulatePrincipalPolicy and falls back to EC2 DryRun calls; a capability with no probe
// is reported unverified. Nothing is created or changed in the account.
func (p *AWSProvider) ProbeCapabilities(ctx context.Context, t Target, names ...string) ([]CapabilityResult, error) {
	caps, err := ResolveCapabilities(names)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, oracleCheckTimeout())
	defer cancel()
	cfg, _, err := p.session(ctx, t)
	if err != nil {
		return nil, err
	}
	id, _, err := p.identity(ctx, cfg)
	if err != nil {
		return nil, err
	}
	var res []CapabilityResult
	if src := principalARN(id.ARN); src != "" {
		sim := p.simulator(cfg)
		if got, serr := sim.Simulate(ctx, simulateRequest{
			PolicySource: src, Actions: allActions(caps), Region: t.Connection.Config.Region,
			TagKey: DefaultPolicyOptions().TagKey,
		}); serr == nil {
			res = fromSimulation(caps, got)
		}
	}
	if res == nil {
		res = p.fromDryRun(ctx, cfg, caps)
	}
	return res, nil
}

func (p *AWSProvider) simulator(cfg aws.Config) PolicySimulator {
	if p.opts.NewSimulator != nil {
		return p.opts.NewSimulator(cfg)
	}
	return newIAMSimulator(cfg)
}

// principalARN turns an assumed-role session ARN into the role ARN SimulatePrincipalPolicy needs.
func principalARN(arn string) string {
	parts := strings.SplitN(arn, ":", 6)
	if len(parts) != 6 || parts[0] != "arn" {
		return ""
	}
	res := strings.Split(parts[5], "/")
	switch {
	case parts[2] == "sts" && len(res) >= 3 && res[0] == "assumed-role":
		return fmt.Sprintf("arn:%s:iam::%s:role/%s", parts[1], parts[4], res[1])
	case parts[2] == "iam" && len(res) >= 2 && (res[0] == "role" || res[0] == "user"):
		return arn
	}
	return ""
}

func allActions(caps []Capability) []string {
	seen := map[string]bool{}
	for _, c := range caps {
		for _, a := range c.ActionList() {
			seen[a] = true
		}
	}
	out := make([]string, 0, len(seen))
	for a := range seen {
		out = append(out, a)
	}
	sort.Strings(out)
	return out
}

func fromSimulation(caps []Capability, decisions map[string]string) []CapabilityResult {
	var out []CapabilityResult
	for _, c := range caps {
		if c.Always {
			continue
		}
		r := CapabilityResult{Name: c.Name, Verified: true, Method: ProbeSimulate}
		for _, a := range c.ActionList() {
			if decisions[a] != "allowed" {
				r.Missing = append(r.Missing, a)
			}
		}
		if r.OK = len(r.Missing) == 0; !r.OK {
			r.Hint = c.Hint
		}
		out = append(out, r)
	}
	return out
}

func (p *AWSProvider) fromDryRun(ctx context.Context, cfg aws.Config, caps []Capability) []CapabilityResult {
	api := p.newEC2(cfg)
	var out []CapabilityResult
	for _, c := range caps {
		if c.Always {
			continue
		}
		r := CapabilityResult{Name: c.Name, Method: ProbeNone, Hint: c.Hint}
		if c.DryRun != "" {
			action := "ec2:" + c.DryRun
			switch dryRun(ctx, api, c.DryRun) {
			case dryAllowed:
				r.OK, r.Verified, r.Method, r.Hint = true, true, ProbeDryRun, ""
			case dryDenied:
				r.Verified, r.Method, r.Missing = true, ProbeDryRun, []string{action}
			}
		}
		out = append(out, r)
	}
	return out
}

type dryOutcome int

const (
	dryUnknown dryOutcome = iota
	dryAllowed
	dryDenied
)

// dryRun relies on EC2 answering DryRunOperation when the call would have been allowed.
func dryRun(ctx context.Context, api EC2API, probe string) dryOutcome {
	var err error
	switch probe {
	case "DescribeInstances":
		_, err = api.DescribeInstances(ctx, &ec2.DescribeInstancesInput{DryRun: aws.Bool(true)})
	case "CreateSecurityGroup":
		_, err = api.CreateSecurityGroup(ctx, &ec2.CreateSecurityGroupInput{
			DryRun: aws.Bool(true), GroupName: aws.String(dryRunGroupName()), Description: aws.String("levelrail permission probe"),
			TagSpecifications: []ec2types.TagSpecification{{
				ResourceType: ec2types.ResourceTypeSecurityGroup,
				Tags:         []ec2types.Tag{{Key: aws.String(DefaultPolicyOptions().TagKey), Value: aws.String(DefaultPolicyOptions().TagValue)}},
			}},
		})
	default:
		return dryUnknown
	}
	switch awsCode(err) {
	case "DryRunOperation":
		return dryAllowed
	case "UnauthorizedOperation", "AccessDenied":
		return dryDenied
	}
	return dryUnknown
}

func dryRunGroupName() string {
	if v := strings.TrimSpace(os.Getenv("APP_CLOUD_AWS_PROBE_GROUP_NAME")); v != "" {
		return v
	}
	return "levelrail-permission-probe"
}
