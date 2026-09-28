package provision

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// ec2API is the subset of *ec2.Client this package calls, narrowed to a
// consumer-defined interface so tests inject a fake with no network I/O,
// the same reasoning internal/api's NodeProvisioner doc comment gives
// for that package's own narrow interface.
type ec2API interface {
	DescribeRegions(ctx context.Context, in *ec2.DescribeRegionsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeRegionsOutput, error)
	DescribeImages(ctx context.Context, in *ec2.DescribeImagesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error)
	DescribeVpcs(ctx context.Context, in *ec2.DescribeVpcsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeVpcsOutput, error)
	DescribeSubnets(ctx context.Context, in *ec2.DescribeSubnetsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSubnetsOutput, error)
	DescribeSecurityGroups(ctx context.Context, in *ec2.DescribeSecurityGroupsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSecurityGroupsOutput, error)
	CreateSecurityGroup(ctx context.Context, in *ec2.CreateSecurityGroupInput, optFns ...func(*ec2.Options)) (*ec2.CreateSecurityGroupOutput, error)
	RunInstances(ctx context.Context, in *ec2.RunInstancesInput, optFns ...func(*ec2.Options)) (*ec2.RunInstancesOutput, error)
	DescribeInstances(ctx context.Context, in *ec2.DescribeInstancesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error)
	TerminateInstances(ctx context.Context, in *ec2.TerminateInstancesInput, optFns ...func(*ec2.Options)) (*ec2.TerminateInstancesOutput, error)
}

var awsRegionDisplayNames = map[string]string{
	"us-east-1":      "US East (N. Virginia)",
	"us-east-2":      "US East (Ohio)",
	"us-west-1":      "US West (N. California)",
	"us-west-2":      "US West (Oregon)",
	"eu-west-1":      "Europe (Ireland)",
	"eu-west-2":      "Europe (London)",
	"eu-west-3":      "Europe (Paris)",
	"eu-central-1":   "Europe (Frankfurt)",
	"eu-north-1":     "Europe (Stockholm)",
	"ap-southeast-1": "Asia Pacific (Singapore)",
	"ap-southeast-2": "Asia Pacific (Sydney)",
	"ap-northeast-1": "Asia Pacific (Tokyo)",
	"ap-south-1":     "Asia Pacific (Mumbai)",
	"sa-east-1":      "South America (Sao Paulo)",
	"ca-central-1":   "Canada (Central)",
}

func awsRegionDisplayName(id string) string {
	if name, ok := awsRegionDisplayNames[id]; ok {
		return name
	}
	return id
}

const (
	awsCanonicalOwnerID  = "099720109477"
	awsUbuntuNamePattern = "ubuntu/images/hvm-ssd-gp3/ubuntu-noble-24.04-amd64-server-*"
)

// findUbuntuAMI resolves the current Ubuntu 24.04 LTS AMI in this
// client's region, the same "current Ubuntu LTS" target
// hetznerImage/digitaloceanImage hardcode as a named image slug: AWS has
// no such friendly name, so this is a live lookup against Canonical's
// own published images instead of a value that would go stale.
func findUbuntuAMI(ctx context.Context, client ec2API) (imageID, rootDevice string, err error) {
	out, err := client.DescribeImages(ctx, &ec2.DescribeImagesInput{
		Owners: []string{awsCanonicalOwnerID},
		Filters: []types.Filter{
			{Name: aws.String("name"), Values: []string{awsUbuntuNamePattern}},
			{Name: aws.String("state"), Values: []string{"available"}},
			{Name: aws.String("virtualization-type"), Values: []string{"hvm"}},
		},
	})
	if err != nil {
		return "", "", fmt.Errorf("find ubuntu ami: %w", err)
	}
	if len(out.Images) == 0 {
		return "", "", fmt.Errorf("no ubuntu 24.04 lts ami found in this region")
	}
	sort.Slice(out.Images, func(i, j int) bool {
		return aws.ToString(out.Images[i].CreationDate) > aws.ToString(out.Images[j].CreationDate)
	})
	img := out.Images[0]
	root := aws.ToString(img.RootDeviceName)
	if root == "" {
		root = "/dev/sda1"
	}
	return aws.ToString(img.ImageId), root, nil
}

// findDefaultVPC requires the account to have a default VPC in this
// region: CreateOpts (shared across every Provisioner) has no field to
// name a specific VPC, and AWS accounts created since December 2013 get
// a default VPC automatically, so this is a documented requirement
// rather than a configurable one for now (docs/node-provisioning.md).
func findDefaultVPC(ctx context.Context, client ec2API) (string, error) {
	out, err := client.DescribeVpcs(ctx, &ec2.DescribeVpcsInput{
		Filters: []types.Filter{{Name: aws.String("isDefault"), Values: []string{"true"}}},
	})
	if err != nil {
		return "", fmt.Errorf("describe vpcs: %w", err)
	}
	if len(out.Vpcs) == 0 {
		return "", fmt.Errorf("no default vpc found in this region; aws provisioning currently requires one")
	}
	return aws.ToString(out.Vpcs[0].VpcId), nil
}

func findDefaultSubnet(ctx context.Context, client ec2API, vpcID string) (string, error) {
	out, err := client.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{
		Filters: []types.Filter{
			{Name: aws.String("vpc-id"), Values: []string{vpcID}},
			{Name: aws.String("default-for-az"), Values: []string{"true"}},
		},
	})
	if err != nil {
		return "", fmt.Errorf("describe subnets: %w", err)
	}
	if len(out.Subnets) == 0 {
		return "", fmt.Errorf("no default subnet found in vpc %q", vpcID)
	}
	return aws.ToString(out.Subnets[0].SubnetId), nil
}

// awsManagedTagValue is the fallback value tagged onto every resource
// this provisioner creates when shortName is empty (brand.ShortName
// unset): a generic, non-brand token, the same reasoning
// internal/network's interfaceName falls back to "wg" for.
const awsManagedTagValue = "app"

// awsManagedTagKey is deliberately a generic key, not a brand-specific
// one: the brand-derived part is the tag's value, not its name (section
// 3's brand indirection rule).
const awsManagedTagKey = "ManagedBy"

func awsShortNameOrFallback(shortName string) string {
	if shortName == "" {
		return awsManagedTagValue
	}
	return shortName
}

// awsSecurityGroupName derives a valid, stable EC2 security group name
// from shortName so repeated calls find and reuse the same group rather
// than creating a new one every time.
func awsSecurityGroupName(shortName string) string {
	clean := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			return r
		case r >= 'A' && r <= 'Z':
			return r + ('a' - 'A')
		default:
			return -1
		}
	}, shortName)
	if clean == "" {
		clean = awsManagedTagValue
	}
	return clean + "-node"
}

// ensureSecurityGroup finds or creates the shared security group every
// AWS-provisioned instance uses: outbound only, no ingress rules at all
// (the agent always dials out, never accepts inbound, see
// docs/node-provisioning.md), matching this platform's "no inbound
// ports on managed servers" architecture (CLAUDE.md 4.3). Optional SSH
// inbound for manual access is not wired here: CreateOpts (shared across
// every Provisioner) has no field to opt into it, so it stays off,
// which is also the requested default.
func ensureSecurityGroup(ctx context.Context, client ec2API, vpcID, shortName string) (string, error) {
	name := awsSecurityGroupName(shortName)
	describeOut, err := client.DescribeSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{
		Filters: []types.Filter{
			{Name: aws.String("group-name"), Values: []string{name}},
			{Name: aws.String("vpc-id"), Values: []string{vpcID}},
		},
	})
	if err != nil {
		return "", fmt.Errorf("describe security groups: %w", err)
	}
	if len(describeOut.SecurityGroups) > 0 {
		return aws.ToString(describeOut.SecurityGroups[0].GroupId), nil
	}
	createOut, err := client.CreateSecurityGroup(ctx, &ec2.CreateSecurityGroupInput{
		GroupName:   aws.String(name),
		Description: aws.String("Outbound only: no inbound ports, the node agent dials out to the control plane."),
		VpcId:       aws.String(vpcID),
		TagSpecifications: []types.TagSpecification{{
			ResourceType: types.ResourceTypeSecurityGroup,
			Tags:         []types.Tag{{Key: aws.String(awsManagedTagKey), Value: aws.String(awsShortNameOrFallback(shortName))}},
		}},
	})
	if err != nil {
		return "", fmt.Errorf("create security group: %w", err)
	}
	return aws.ToString(createOut.GroupId), nil
}

// encodeAWSServerID packs region into the returned server id: GetServer
// and DeleteServer only take an id, but an EC2 instance id is only
// valid in the region it was created in, so the id this provisioner
// hands back has to carry that region itself. Opaque to every caller,
// which only ever stores and replays it verbatim (store.NodeProvision's
// own ProviderServerID column, internal/api/node_provision.go).
func encodeAWSServerID(region, instanceID string) string {
	return region + "/" + instanceID
}

func decodeAWSServerID(id string) (region, instanceID string, err error) {
	region, instanceID, ok := strings.Cut(id, "/")
	if !ok || region == "" || instanceID == "" {
		return "", "", fmt.Errorf("malformed aws server id %q", id)
	}
	return region, instanceID, nil
}

func awsInstanceStatus(state *types.InstanceState) ServerStatus {
	if state == nil {
		return ServerStatusPending
	}
	switch state.Name {
	case types.InstanceStateNameRunning:
		return ServerStatusRunning
	case types.InstanceStateNameTerminated, types.InstanceStateNameShuttingDown,
		types.InstanceStateNameStopping, types.InstanceStateNameStopped:
		return ServerStatusError
	default:
		// pending
		return ServerStatusPending
	}
}
