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
			Actions: []string{"iam:SimulatePrincipalPolicy", "iam:ListAttachedRolePolicies"}, Resources: []string{"{self_role}"}, Global: true,
		}},
	},
	{
		Name: CapInventory, Summary: "List instances, networks, volumes and regions", DryRun: "DescribeInstances",
		Hint: "Add the inventory tier to the stack and update it.",
		Statements: []CapStatement{
			{Actions: []string{
				"ec2:DescribeInstances", "ec2:DescribeInstanceTypes", "ec2:DescribeImages", "ec2:DescribeVpcs", "ec2:DescribeSubnets",
				"ec2:DescribeSecurityGroups", "ec2:DescribeVolumes", "ec2:DescribeSnapshots", "ec2:DescribeAddresses", "ec2:DescribeKeyPairs", "ec2:DescribeAvailabilityZones",
			}, Resources: []string{all}},
			{Actions: []string{"ec2:DescribeRegions", "tag:GetResources"}, Resources: []string{all}, Global: true},
			// Describe and List actions of these services do not support resource scoping, so they use "*".
			{Actions: []string{
				"rds:DescribeDBInstances", "rds:DescribeDBClusters", "rds:DescribeDBSnapshots", "rds:DescribeDBClusterSnapshots",
				"rds:DescribeDBParameterGroups", "rds:ListTagsForResource", "elasticache:DescribeCacheClusters", "elasticache:DescribeReplicationGroups",
				"elasticache:DescribeSnapshots", "elasticache:ListTagsForResource", "ecr:DescribeRepositories", "ecr:ListImages",
				"acm:ListCertificates", "acm:DescribeCertificate", "ssm:DescribeInstanceInformation",
				"lightsail:GetInstances", "lightsail:GetInstance", "lightsail:GetBundles", "lightsail:GetBlueprints", "lightsail:GetRegions",
				"lightsail:GetInstanceSnapshots", "lightsail:GetStaticIps",
				"cloudtrail:LookupEvents", "cloudwatch:GetMetricData", "cloudwatch:ListMetrics", "logs:DescribeLogGroups", "logs:FilterLogEvents",
			}, Resources: []string{all}},
			// CloudFront is a global service, so its actions skip the region condition.
			{Actions: []string{"cloudfront:ListDistributions", "cloudfront:GetDistribution", "cloudfront:ListInvalidations"}, Resources: []string{all}, Global: true},
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
				Values: []string{"RunInstances", "CreateVolume", "CreateSecurityGroup", "CreateKeyPair", "AllocateAddress", "CreateSnapshot", "CreateSnapshots"},
			}}},
			// Snapshots are tagged on create; the source volume or instance must carry the managed tag.
			{Actions: []string{"ec2:CreateSnapshot", "ec2:CreateSnapshots"}, Resources: []string{"arn:*:ec2:*::snapshot/*"}, Tag: TagRequest},
			{Actions: []string{"ec2:CreateSnapshot", "ec2:CreateSnapshots"}, Resources: []string{"arn:*:ec2:*:*:volume/*", "arn:*:ec2:*:*:instance/*"}, Tag: TagResource},
			// Create actions need the request tag; supporting resources (subnet group, parameter and option groups) have no tag key of their own.
			{Actions: []string{
				"rds:CreateDBInstance", "rds:CreateDBCluster", "rds:CreateDBSnapshot", "rds:CreateDBClusterSnapshot", "rds:CopyDBSnapshot", "rds:AddTagsToResource",
			}, Resources: []string{
				"arn:*:rds:*:*:db:*", "arn:*:rds:*:*:cluster:*", "arn:*:rds:*:*:snapshot:*", "arn:*:rds:*:*:cluster-snapshot:*",
				"arn:*:rds:*:*:subgrp:*", "arn:*:rds:*:*:pg:*", "arn:*:rds:*:*:cluster-pg:*", "arn:*:rds:*:*:og:*",
			}, Tag: TagRequest},
			{Actions: []string{"rds:ModifyDBInstance", "rds:StartDBInstance", "rds:StopDBInstance", "rds:StartDBCluster", "rds:StopDBCluster"},
				Resources: []string{"arn:*:rds:*:*:db:*", "arn:*:rds:*:*:cluster:*"}, Tag: TagResource},
			{Actions: []string{"elasticache:CreateSnapshot", "elasticache:AddTagsToResource"},
				Resources: []string{"arn:*:elasticache:*:*:cluster:*", "arn:*:elasticache:*:*:replicationgroup:*", "arn:*:elasticache:*:*:snapshot:*"}, Tag: TagRequest},
			{Actions: []string{"elasticache:RebootCacheCluster"}, Resources: []string{"arn:*:elasticache:*:*:cluster:*"}, Tag: TagResource},
			// Lightsail creates carry the request tag; power changes and snapshots need the managed tag on the instance.
			{Actions: []string{"lightsail:CreateInstances", "lightsail:CreateInstanceSnapshot", "lightsail:TagResource"},
				Resources: []string{"arn:*:lightsail:*:*:Instance/*", "arn:*:lightsail:*:*:InstanceSnapshot/*"}, Tag: TagRequest},
			{Actions: []string{"lightsail:StartInstance", "lightsail:StopInstance", "lightsail:RebootInstance", "lightsail:CreateInstanceSnapshot"},
				Resources: []string{"arn:*:lightsail:*:*:Instance/*"}, Tag: TagResource},
			{Actions: []string{"ecr:CreateRepository", "ecr:TagResource"}, Resources: []string{"arn:*:ecr:*:*:repository/*"}, Tag: TagRequest},
			{Actions: []string{"ecr:PutLifecyclePolicy"}, Resources: []string{"arn:*:ecr:*:*:repository/*"}, Tag: TagResource},
			// GetAuthorizationToken cannot be scoped to a resource.
			{Actions: []string{"ecr:GetAuthorizationToken"}, Resources: []string{all}},
			// RequestCertificate has no resource type, so it is scoped by request tag only.
			{Actions: []string{"acm:RequestCertificate", "acm:AddTagsToCertificate"}, Resources: []string{all}, Tag: TagRequest},
			{Actions: []string{"cloudfront:CreateInvalidation"}, Resources: []string{"arn:*:cloudfront::*:distribution/*"}, Global: true},
			{Actions: []string{"secretsmanager:ListSecrets"}, Resources: []string{all}},
			// Secrets and parameters are limited to the {name_prefix}/ name prefix (setting cloud.aws.name_prefix); RDS managed passwords use the rds! prefix.
			{Actions: []string{"secretsmanager:GetSecretValue", "secretsmanager:PutSecretValue"}, Resources: []string{"arn:*:secretsmanager:*:*:secret:{name_prefix}/*"}},
			{Actions: []string{"secretsmanager:CreateSecret", "secretsmanager:TagResource"}, Resources: []string{"arn:*:secretsmanager:*:*:secret:rds!*"}},
			{Actions: []string{"ssm:GetParametersByPath", "ssm:PutParameter"}, Resources: []string{"arn:*:ssm:*:*:parameter/{name_prefix}/*"}},
			// RDS managed master passwords resolve the key through DescribeKey, which has no tag key.
			{Actions: []string{"kms:DescribeKey"}, Resources: []string{all}},
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
			{Actions: []string{"s3:CreateBucket", "s3:TagResource"}, Resources: []string{"arn:*:s3:::{bucket_prefix}*"}, Tag: TagRequest, Global: true},
			// S3 ABAC is off per bucket by default, so bucket access is scoped by name prefix, not tag.
			{Actions: []string{"s3:ListBucket", "s3:GetBucketLocation", "s3:GetBucketTagging"}, Resources: []string{"arn:*:s3:::{bucket_prefix}*"}, Global: true},
			{Actions: []string{"s3:GetObject", "s3:PutObject", "s3:AbortMultipartUpload", "s3:ListMultipartUploadParts"}, Resources: []string{"arn:*:s3:::{bucket_prefix}*/*"}, Global: true},
			{Actions: []string{"s3:UntagResource"}, Resources: []string{"arn:*:s3:::{bucket_prefix}*"}, Global: true},
		},
	},
	{
		Name: CapDNS, Mutating: true, Summary: "Read zones and change records in the listed zones", Hint: "Add the dns tier to the stack and update it.",
		Statements: []CapStatement{
			{Actions: []string{"route53:ListHostedZones", "route53:ListHostedZonesByName", "route53:GetChange"}, Resources: []string{all}, Global: true},
			// The hostedzone resource type has no condition keys, so zones are scoped by ARN.
			{Actions: []string{"route53:GetHostedZone", "route53:ListResourceRecordSets", "route53:ChangeResourceRecordSets"}, Resources: []string{"{hosted_zone_ids}"}, Global: true},
		},
	},
	{
		Name: CapDestructive, Mutating: true, Summary: "Delete tagged resources and the listed zones", Hint: "Add the destructive tier only when Levelrail should delete resources.",
		Statements: []CapStatement{
			{Actions: []string{
				"ec2:TerminateInstances", "ec2:DeleteVolume", "ec2:DeleteSecurityGroup", "ec2:DeleteKeyPair", "ec2:ReleaseAddress", "ec2:DetachVolume",
			}, Resources: []string{all}, Tag: TagResource},
			{Actions: []string{"s3:DeleteBucket"}, Resources: []string{"arn:*:s3:::{bucket_prefix}*"}, Global: true},
			{Actions: []string{"s3:DeleteObject"}, Resources: []string{"arn:*:s3:::{bucket_prefix}*/*"}, Global: true},
			{Actions: []string{"route53:DeleteHostedZone"}, Resources: []string{"{hosted_zone_ids}"}, Global: true},
			{Actions: []string{"ec2:DeleteSnapshot"}, Resources: []string{"arn:*:ec2:*::snapshot/*"}, Tag: TagResource},
			{Actions: []string{"rds:DeleteDBInstance", "rds:DeleteDBSnapshot", "rds:DeleteDBClusterSnapshot"},
				Resources: []string{"arn:*:rds:*:*:db:*", "arn:*:rds:*:*:snapshot:*", "arn:*:rds:*:*:cluster-snapshot:*"}, Tag: TagResource},
			{Actions: []string{"elasticache:DeleteCacheCluster", "elasticache:DeleteReplicationGroup"},
				Resources: []string{"arn:*:elasticache:*:*:cluster:*", "arn:*:elasticache:*:*:replicationgroup:*"}, Tag: TagResource},
			// Final snapshots on delete are created and tagged by the delete call itself.
			{Actions: []string{"elasticache:CreateSnapshot", "rds:CreateDBSnapshot", "rds:CreateDBClusterSnapshot", "rds:AddTagsToResource"},
				Resources: []string{"arn:*:elasticache:*:*:snapshot:*", "arn:*:rds:*:*:snapshot:*", "arn:*:rds:*:*:cluster-snapshot:*"}, Tag: TagRequest},
			{Actions: []string{"lightsail:DeleteInstance", "lightsail:DeleteInstanceSnapshot"},
				Resources: []string{"arn:*:lightsail:*:*:Instance/*", "arn:*:lightsail:*:*:InstanceSnapshot/*"}, Tag: TagResource},
			{Actions: []string{"ecr:DeleteRepository"}, Resources: []string{"arn:*:ecr:*:*:repository/*"}, Tag: TagResource},
			// Remote command and shell access run code on hosts, so only this tier grants them. Documents have no tag key, instances do.
			{Actions: []string{"ssm:SendCommand"}, Resources: []string{"arn:*:ssm:*:*:document/AWS-RunShellScript"}},
			{Actions: []string{"ssm:SendCommand"}, Resources: []string{"arn:*:ec2:*:*:instance/*"}, Tag: TagResource},
			{Actions: []string{"ssm:StartSession"}, Resources: []string{"arn:*:ssm:*:*:document/SSM-SessionManagerRunShell"}},
			{Actions: []string{"ssm:StartSession"}, Resources: []string{"arn:*:ec2:*:*:instance/*"}, Tag: TagResource},
			{Actions: []string{"ssm:GetCommandInvocation"}, Resources: []string{all}},
		},
	},
}

// awsBoundaryDeny is denied in the permission boundary so the role can never change identities,
// whatever a tier grants. iam:SimulatePrincipalPolicy stays allowed.
var awsBoundaryDeny = []string{
	"iam:Create*", "iam:Delete*", "iam:Put*", "iam:Attach*", "iam:Detach*", "iam:Update*", "iam:Add*", "iam:Remove*",
	"iam:Set*", "iam:Tag*", "iam:Untag*", "iam:Upload*", "iam:Enable*", "iam:Disable*", "organizations:*", "account:*",
}

// awsBoundaryDenyScoped blocks actions inside allowed services that could widen exposure or hide
// activity: sharing snapshots, resource policies, run documents, audit and log changes.
var awsBoundaryDenyScoped = []string{
	"rds:ModifyDBSnapshotAttribute", "rds:ModifyDBClusterSnapshotAttribute", "secretsmanager:PutResourcePolicy", "ecr:SetRepositoryPolicy",
	"ssm:CreateDocument", "ssm:UpdateDocument", "ssm:CreateAssociation", "cloudtrail:StopLogging", "cloudtrail:DeleteTrail",
	"cloudtrail:UpdateTrail", "cloudtrail:PutEventSelectors", "logs:DeleteLogGroup", "logs:PutSubscriptionFilter", "cloudfront:UpdateDistribution",
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
			TagKey: DefaultPolicyOptions().TagKey, Resources: representativeResources(caps, DefaultPolicyOptions(), src, t.Connection.Config.Region),
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

// representativeResources maps each action to concrete example ARNs of the statements that
// grant it, so scoped statements are not reported as implicit denies against "*".
func representativeResources(caps []Capability, opt PolicyOptions, principal, region string) map[string][]string {
	parts := strings.SplitN(principal, ":", 6)
	if len(parts) != 6 {
		return nil
	}
	partition, account := parts[1], parts[4]
	out := map[string][]string{}
	for _, doc := range renderCaps(caps, opt).Statement {
		for _, a := range doc.Action {
			for _, r := range doc.Resource {
				c := concreteARN(r, partition, region, account)
				if !containsString(out[a], c) {
					out[a] = append(out[a], c)
				}
			}
		}
	}
	return out
}

func containsString(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// concreteARN fills wildcard segments of a policy resource with example values.
func concreteARN(pattern, partition, region, account string) string {
	if pattern == all {
		return all
	}
	parts := strings.SplitN(pattern, ":", 6)
	if len(parts) != 6 {
		return pattern
	}
	parts[1] = partition
	if parts[3] == "*" && parts[2] != "s3" && parts[2] != "route53" && parts[2] != "iam" {
		parts[3] = region
	}
	if parts[4] == "*" && parts[2] != "s3" && parts[2] != "route53" {
		parts[4] = account
	}
	res := parts[5]
	switch {
	case strings.Contains(res, "/*") && strings.HasSuffix(res, "/*"):
		res = strings.TrimSuffix(res, "*") + "example"
		if parts[2] == "s3" {
			res = strings.Replace(res, "*/", "example/", 1)
		}
	case strings.HasSuffix(res, "*"):
		res = strings.TrimSuffix(res, "*") + "example"
	}
	parts[5] = res
	return strings.Join(parts, ":")
}
