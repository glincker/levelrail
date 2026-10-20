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
	AuthorizeSecurityGroupIngress(ctx context.Context, in *ec2.AuthorizeSecurityGroupIngressInput, optFns ...func(*ec2.Options)) (*ec2.AuthorizeSecurityGroupIngressOutput, error)
	RevokeSecurityGroupIngress(ctx context.Context, in *ec2.RevokeSecurityGroupIngressInput, optFns ...func(*ec2.Options)) (*ec2.RevokeSecurityGroupIngressOutput, error)
	DeleteSecurityGroup(ctx context.Context, in *ec2.DeleteSecurityGroupInput, optFns ...func(*ec2.Options)) (*ec2.DeleteSecurityGroupOutput, error)
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

// awsSanitizeGroupNameComponent lowercases and strips shortName (or any
// other free-text component) down to the characters valid in an EC2
// security group name.
func awsSanitizeGroupNameComponent(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			return r
		case r >= 'A' && r <= 'Z':
			return r + ('a' - 'A')
		default:
			return -1
		}
	}, s)
}

// awsSecurityGroupName derives a valid, stable EC2 security group name
// from shortName so repeated calls find and reuse the same group rather
// than creating a new one every time.
func awsSecurityGroupName(shortName string) string {
	clean := awsSanitizeGroupNameComponent(shortName)
	if clean == "" {
		clean = awsManagedTagValue
	}
	return clean + "-node"
}

// awsDedicatedSecurityGroupName derives a per-instance group name from
// shortName and the instance's own (already name-collision-checked)
// name, distinct from awsSecurityGroupName's shared group.
func awsDedicatedSecurityGroupName(shortName, instanceName string) string {
	cleanShort := awsSanitizeGroupNameComponent(shortName)
	if cleanShort == "" {
		cleanShort = awsManagedTagValue
	}
	cleanName := awsSanitizeGroupNameComponent(instanceName)
	if cleanName == "" {
		cleanName = "instance"
	}
	return cleanShort + "-" + cleanName + "-ssh"
}

// ensureSecurityGroup finds or creates the shared security group every
// AWS-provisioned instance uses by default: outbound only, no ingress
// rules at all (the agent always dials out, never accepts inbound, see
// docs/node-provisioning.md), matching this platform's "no inbound
// ports on managed servers" architecture (CLAUDE.md 4.3).
// CreateOpts.AllowSSHInbound opts a specific instance out of this shared
// group into its own dedicated one instead, see
// ensureDedicatedSSHSecurityGroup.
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
		sg := describeOut.SecurityGroups[0]
		if err := revokeUnexpectedIngress(ctx, client, sg); err != nil {
			return "", err
		}
		return aws.ToString(sg.GroupId), nil
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

// revokeUnexpectedIngress keeps a reused shared security group converged on
// ensureSecurityGroup's own "no inbound ports" invariant: if an operator
// (or anything else) has added ingress rules to a group with the derived
// name, reusing it as-is would silently attach those rules to every future
// node. Revoking on reuse, not just on creation, is what a level-triggered
// reconciler does with any other drifted resource.
func revokeUnexpectedIngress(ctx context.Context, client ec2API, sg types.SecurityGroup) error {
	if len(sg.IpPermissions) == 0 {
		return nil
	}
	if _, err := client.RevokeSecurityGroupIngress(ctx, &ec2.RevokeSecurityGroupIngressInput{
		GroupId:       sg.GroupId,
		IpPermissions: sg.IpPermissions,
	}); err != nil {
		return fmt.Errorf("revoke unexpected ingress on reused security group %q: %w", aws.ToString(sg.GroupId), err)
	}
	return nil
}

// awsDedicatedSecurityGroupTagKey marks a security group as created
// dedicated to one instance (CreateOpts.AllowSSHInbound), distinct from
// the shared, reused group ensureSecurityGroup manages: only a group
// carrying this tag is ever a candidate for deletion in AWS.DeleteServer.
const awsDedicatedSecurityGroupTagKey = "DedicatedSecurityGroup"

const awsSSHPort int32 = 22

// ensureDedicatedSSHSecurityGroup finds or creates a security group
// scoped to one instance (awsDedicatedSecurityGroupName), with TCP 22
// open to any source: there is no per-operator SSH key or source-IP
// input yet, so this is deliberately opt-in (CreateOpts.AllowSSHInbound)
// rather than a default. Tagged so AWS.DeleteServer can tell it apart
// from the shared no-inbound group and safely delete it once the
// instance it belongs to is gone.
func ensureDedicatedSSHSecurityGroup(ctx context.Context, client ec2API, vpcID, shortName, instanceName string) (string, error) {
	name := awsDedicatedSecurityGroupName(shortName, instanceName)
	describeOut, err := client.DescribeSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{
		Filters: []types.Filter{
			{Name: aws.String("group-name"), Values: []string{name}},
			{Name: aws.String("vpc-id"), Values: []string{vpcID}},
		},
	})
	if err != nil {
		return "", fmt.Errorf("describe dedicated security group: %w", err)
	}
	if len(describeOut.SecurityGroups) > 0 {
		return aws.ToString(describeOut.SecurityGroups[0].GroupId), nil
	}
	createOut, err := client.CreateSecurityGroup(ctx, &ec2.CreateSecurityGroupInput{
		GroupName:   aws.String(name),
		Description: aws.String("Dedicated to one provisioned instance: SSH inbound opted in at creation, deleted with the instance."),
		VpcId:       aws.String(vpcID),
		TagSpecifications: []types.TagSpecification{{
			ResourceType: types.ResourceTypeSecurityGroup,
			Tags: []types.Tag{
				{Key: aws.String(awsManagedTagKey), Value: aws.String(awsShortNameOrFallback(shortName))},
				{Key: aws.String(awsDedicatedSecurityGroupTagKey), Value: aws.String("true")},
			},
		}},
	})
	if err != nil {
		return "", fmt.Errorf("create dedicated security group: %w", err)
	}
	groupID := aws.ToString(createOut.GroupId)
	if _, err := client.AuthorizeSecurityGroupIngress(ctx, &ec2.AuthorizeSecurityGroupIngressInput{
		GroupId: aws.String(groupID),
		IpPermissions: []types.IpPermission{{
			IpProtocol: aws.String("tcp"),
			FromPort:   aws.Int32(awsSSHPort),
			ToPort:     aws.Int32(awsSSHPort),
			IpRanges:   []types.IpRange{{CidrIp: aws.String("0.0.0.0/0"), Description: aws.String("SSH, opted in at provision time")}},
		}},
	}); err != nil {
		return "", fmt.Errorf("authorize ssh ingress on dedicated security group: %w", err)
	}
	return groupID, nil
}

// instanceSecurityGroupIDs returns the security groups attached to
// instanceID, filtered defensively to that instance's own entries even
// though the DescribeInstances call already scopes to it: best-effort,
// errors are swallowed (nil) since a failure here should not block
// AWS.DeleteServer from still terminating the instance itself.
func instanceSecurityGroupIDs(ctx context.Context, client ec2API, instanceID string) []string {
	out, err := client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{InstanceIds: []string{instanceID}})
	if err != nil || out == nil {
		return nil
	}
	var ids []string
	for _, res := range out.Reservations {
		for _, inst := range res.Instances {
			if aws.ToString(inst.InstanceId) != instanceID {
				continue
			}
			for _, sg := range inst.SecurityGroups {
				ids = append(ids, aws.ToString(sg.GroupId))
			}
		}
	}
	return ids
}

// securityGroupIsDedicated reports whether groupID carries
// awsDedicatedSecurityGroupTagKey, i.e. was created by
// ensureDedicatedSSHSecurityGroup for one specific instance rather than
// shared across every AWS-provisioned node.
func securityGroupIsDedicated(ctx context.Context, client ec2API, groupID string) bool {
	out, err := client.DescribeSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{GroupIds: []string{groupID}})
	if err != nil || out == nil {
		return false
	}
	for _, sg := range out.SecurityGroups {
		if aws.ToString(sg.GroupId) != groupID {
			continue
		}
		for _, t := range sg.Tags {
			if aws.ToString(t.Key) == awsDedicatedSecurityGroupTagKey && aws.ToString(t.Value) == "true" {
				return true
			}
		}
	}
	return false
}

// securityGroupOtherInstanceCount counts non-terminated instances that
// reference groupID, excluding excludeInstanceID (the one DeleteServer
// just terminated): a dedicated group is only ever safe to delete when
// this comes back zero.
func securityGroupOtherInstanceCount(ctx context.Context, client ec2API, groupID, excludeInstanceID string) int {
	out, err := client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		Filters: []types.Filter{
			{Name: aws.String("instance.group-id"), Values: []string{groupID}},
			{Name: aws.String("instance-state-name"), Values: []string{"pending", "running", "shutting-down", "stopping", "stopped"}},
		},
	})
	if err != nil || out == nil {
		// Unknown is treated as "in use": erring toward leaving a group
		// behind (a cosmetic, non-billable leftover) rather than risking
		// deletion of one another instance still depends on.
		return 1
	}
	count := 0
	for _, res := range out.Reservations {
		for _, inst := range res.Instances {
			if aws.ToString(inst.InstanceId) == excludeInstanceID {
				continue
			}
			count++
		}
	}
	return count
}

// cleanupDedicatedSecurityGroups deletes every group in groupIDs that is
// both dedicated (securityGroupIsDedicated) and no longer referenced by
// any other instance. Best-effort and silent: called after
// TerminateInstances already succeeded, so a security group still
// attached to a not-yet-detached network interface during the
// instance's shutting-down window is an expected, transient failure, not
// one worth surfacing on top of an otherwise successful delete.
func cleanupDedicatedSecurityGroups(ctx context.Context, client ec2API, terminatedInstanceID string, groupIDs []string) {
	for _, groupID := range groupIDs {
		if !securityGroupIsDedicated(ctx, client, groupID) {
			continue
		}
		if securityGroupOtherInstanceCount(ctx, client, groupID, terminatedInstanceID) > 0 {
			continue
		}
		_, _ = client.DeleteSecurityGroup(ctx, &ec2.DeleteSecurityGroupInput{GroupId: aws.String(groupID)})
	}
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
