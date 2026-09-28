package provision

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

const awsDefaultRegion = "us-east-1"

// awsDefaultDiskGB is the root EBS volume size every provisioned
// instance gets. Unlike Hetzner/DigitalOcean, EC2 decouples disk from
// instance type, so this isn't derived from the chosen size.
const awsDefaultDiskGB = 20

// AWSCredentials is the JSON shape node-provider/aws's stored credential
// value holds under NodeProviderTokenEnvKey (store.NodeProviderSecretsKey):
// EC2 has no single bearer token the way Hetzner and DigitalOcean do, so
// that envKey carries this struct JSON-encoded instead of a raw string.
type AWSCredentials struct {
	AccessKeyID     string `json:"access_key_id,omitempty"`
	SecretAccessKey string `json:"secret_access_key,omitempty"`
	SessionToken    string `json:"session_token,omitempty"`
	Region          string `json:"region,omitempty"`
	// RoleARN, when set, is assumed via STS AssumeRole on top of
	// whichever base credentials resolve below: the base credentials
	// then only need sts:AssumeRole on this one ARN, not direct EC2
	// permissions, matching least-privilege IAM practice.
	RoleARN string `json:"role_arn,omitempty"`
	// UseAmbientCredentials, with AccessKeyID/SecretAccessKey both
	// empty, resolves this control plane's own AWS identity (env vars,
	// shared config, or an EC2 instance profile) instead of a stored
	// static key: only meaningful when the control plane itself runs on
	// AWS. Full AssumeRoleWithWebIdentity (OIDC federation, the way
	// GitHub Actions authenticates to AWS) is not implemented: it needs
	// this control plane to be its own trusted OIDC token issuer, which
	// does not exist. RoleARN plus a static key, or plus this ambient
	// chain, are the two supported ways to avoid a long-lived key with
	// direct EC2 permissions.
	UseAmbientCredentials bool `json:"use_ambient_credentials,omitempty"`
}

func (c AWSCredentials) region() string {
	if c.Region != "" {
		return c.Region
	}
	return awsDefaultRegion
}

func (c AWSCredentials) validate() error {
	if c.UseAmbientCredentials {
		return nil
	}
	if c.AccessKeyID == "" || c.SecretAccessKey == "" {
		return fmt.Errorf("provision: aws credentials require an access key id and secret access key, or use_ambient_credentials")
	}
	return nil
}

// AWS implements Provisioner against the EC2 API (ec2API, aws_network.go),
// the same aws-sdk-go-v2 dependency internal/objectstore (S3) and
// internal/email (SES) already use. Unlike Hetzner/DigitalOcean's single
// bearer token and location-agnostic server id, an EC2 instance id is
// only valid in the region it was created in, so client builds a
// region-scoped client lazily per call rather than storing one fixed
// *ec2.Client.
type AWS struct {
	creds     AWSCredentials
	shortName string
	client    func(ctx context.Context, region string) (ec2API, error)
}

// NewAWS returns an AWS provisioner authenticating with creds.
// shortName namespaces the security group it creates
// (brand.Brand.ShortName; the product name never appears in source, so
// this is passed in, not looked up, the convention
// internal/network.WithShortName already establishes).
func NewAWS(creds AWSCredentials, shortName string) *AWS {
	a := &AWS{creds: creds, shortName: shortName}
	a.client = a.buildClient
	return a
}

// NewAWSFromToken builds an AWS provisioner from the JSON-encoded
// AWSCredentials a NodeProviderSecrets lookup returns.
func NewAWSFromToken(token, shortName string) (*AWS, error) {
	var creds AWSCredentials
	if err := json.Unmarshal([]byte(token), &creds); err != nil {
		return nil, fmt.Errorf("provision: decode aws credentials: %w", err)
	}
	if err := creds.validate(); err != nil {
		return nil, err
	}
	return NewAWS(creds, shortName), nil
}

// buildClient resolves credentials and returns an ec2API scoped to
// region (a.creds.region() when region is empty, for calls like
// ListRegions that aren't yet tied to one).
func (a *AWS) buildClient(ctx context.Context, region string) (ec2API, error) {
	if region == "" {
		region = a.creds.region()
	}
	var credsProvider aws.CredentialsProvider
	if a.creds.UseAmbientCredentials {
		cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
		if err != nil {
			return nil, fmt.Errorf("load ambient aws credentials: %w", err)
		}
		credsProvider = cfg.Credentials
	} else {
		credsProvider = credentials.NewStaticCredentialsProvider(a.creds.AccessKeyID, a.creds.SecretAccessKey, a.creds.SessionToken)
	}
	if a.creds.RoleARN != "" {
		stsClient := sts.New(sts.Options{Region: region, Credentials: credsProvider})
		credsProvider = aws.NewCredentialsCache(stscreds.NewAssumeRoleProvider(stsClient, a.creds.RoleARN))
	}
	return ec2.New(ec2.Options{Region: region, Credentials: credsProvider}), nil
}

// ListRegions calls EC2 DescribeRegions.
func (a *AWS) ListRegions(ctx context.Context) ([]Region, error) {
	client, err := a.client(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("provision: aws list regions: %w", err)
	}
	out, err := client.DescribeRegions(ctx, &ec2.DescribeRegionsInput{})
	if err != nil {
		return nil, fmt.Errorf("provision: aws list regions: %w", err)
	}
	regions := make([]Region, 0, len(out.Regions))
	for _, r := range out.Regions {
		name := aws.ToString(r.RegionName)
		regions = append(regions, Region{ID: name, Name: awsRegionDisplayName(name)})
	}
	return regions, nil
}

// awsCuratedSizes is a curated set of common general-purpose instance
// types, not the full EC2 catalog: EC2 has hundreds of instance types
// across a dozen families, and most don't suit this platform's 1-10
// machine target audience any better than Hetzner/DigitalOcean's own
// short, fixed size lists do. Disk is fixed at awsDefaultDiskGB for all
// of them since EC2 decouples storage from instance type.
var awsCuratedSizes = []Size{
	{ID: "t3.micro", Name: "t3.micro", VCPUs: 2, Memory: 1024, Disk: awsDefaultDiskGB},
	{ID: "t3.small", Name: "t3.small", VCPUs: 2, Memory: 2048, Disk: awsDefaultDiskGB},
	{ID: "t3.medium", Name: "t3.medium", VCPUs: 2, Memory: 4096, Disk: awsDefaultDiskGB},
	{ID: "t3.large", Name: "t3.large", VCPUs: 2, Memory: 8192, Disk: awsDefaultDiskGB},
	{ID: "t3.xlarge", Name: "t3.xlarge", VCPUs: 4, Memory: 16384, Disk: awsDefaultDiskGB},
	{ID: "t3.2xlarge", Name: "t3.2xlarge", VCPUs: 8, Memory: 32768, Disk: awsDefaultDiskGB},
}

// ListSizes returns awsCuratedSizes: a static list, not a live API call
// (unlike Hetzner/DigitalOcean, EC2's own catalog is too large to be a
// useful picker, see awsCuratedSizes). region is accepted to satisfy
// Provisioner but unused: every listed type is available in essentially
// every commercial AWS region.
func (a *AWS) ListSizes(_ context.Context, _ string) ([]Size, error) {
	sizes := make([]Size, len(awsCuratedSizes))
	copy(sizes, awsCuratedSizes)
	return sizes, nil
}

// CreateServer launches an EC2 instance: resolves the current Ubuntu
// AMI, the account's default VPC and subnet, ensures the shared
// no-inbound security group exists, and runs one instance with opts as
// its user-data.
func (a *AWS) CreateServer(ctx context.Context, opts CreateOpts) (serverID, ipAddr string, err error) {
	if opts.Region == "" {
		return "", "", fmt.Errorf("provision: aws create server: region is required")
	}
	client, err := a.client(ctx, opts.Region)
	if err != nil {
		return "", "", fmt.Errorf("provision: aws create server: %w", err)
	}

	amiID, rootDevice, err := findUbuntuAMI(ctx, client)
	if err != nil {
		return "", "", fmt.Errorf("provision: aws create server: %w", err)
	}
	vpcID, err := findDefaultVPC(ctx, client)
	if err != nil {
		return "", "", fmt.Errorf("provision: aws create server: %w", err)
	}
	subnetID, err := findDefaultSubnet(ctx, client, vpcID)
	if err != nil {
		return "", "", fmt.Errorf("provision: aws create server: %w", err)
	}
	var sgID string
	if opts.AllowSSHInbound {
		sgID, err = ensureDedicatedSSHSecurityGroup(ctx, client, vpcID, a.shortName, opts.Name)
	} else {
		sgID, err = ensureSecurityGroup(ctx, client, vpcID, a.shortName)
	}
	if err != nil {
		return "", "", fmt.Errorf("provision: aws create server: %w", err)
	}

	out, err := client.RunInstances(ctx, &ec2.RunInstancesInput{
		ImageId:          aws.String(amiID),
		InstanceType:     types.InstanceType(opts.Size),
		MinCount:         aws.Int32(1),
		MaxCount:         aws.Int32(1),
		UserData:         aws.String(base64.StdEncoding.EncodeToString([]byte(opts.UserData))),
		SubnetId:         aws.String(subnetID),
		SecurityGroupIds: []string{sgID},
		BlockDeviceMappings: []types.BlockDeviceMapping{{
			DeviceName: aws.String(rootDevice),
			Ebs: &types.EbsBlockDevice{
				VolumeSize:          aws.Int32(awsDefaultDiskGB),
				DeleteOnTermination: aws.Bool(true),
			},
		}},
		TagSpecifications: []types.TagSpecification{{
			ResourceType: types.ResourceTypeInstance,
			Tags: []types.Tag{
				{Key: aws.String("Name"), Value: aws.String(opts.Name)},
				{Key: aws.String(awsManagedTagKey), Value: aws.String(awsShortNameOrFallback(a.shortName))},
			},
		}},
	})
	if err != nil {
		return "", "", fmt.Errorf("provision: aws run instances: %w", err)
	}
	if len(out.Instances) == 0 {
		return "", "", fmt.Errorf("provision: aws run instances: no instance returned")
	}
	inst := out.Instances[0]
	return encodeAWSServerID(opts.Region, aws.ToString(inst.InstanceId)), aws.ToString(inst.PublicIpAddress), nil
}

// GetServer calls EC2 DescribeInstances.
func (a *AWS) GetServer(ctx context.Context, id string) (ServerStatus, string, error) {
	region, instanceID, err := decodeAWSServerID(id)
	if err != nil {
		return "", "", fmt.Errorf("provision: aws get server: %w", err)
	}
	client, err := a.client(ctx, region)
	if err != nil {
		return "", "", fmt.Errorf("provision: aws get server %q: %w", id, err)
	}
	out, err := client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{InstanceIds: []string{instanceID}})
	if err != nil {
		return "", "", fmt.Errorf("provision: aws get server %q: %w", id, err)
	}
	for _, res := range out.Reservations {
		for _, inst := range res.Instances {
			return awsInstanceStatus(inst.State), aws.ToString(inst.PublicIpAddress), nil
		}
	}
	return "", "", fmt.Errorf("provision: aws get server %q: not found", id)
}

// DeleteServer terminates the EC2 instance. The shared security group
// ensureSecurityGroup creates or reuses is never deleted here: it's
// shared across every AWS-provisioned node, not created fresh per
// instance, so a single DeleteServer call is never its sole owner and
// cannot safely tell whether other nodes still depend on it. A group
// ensureDedicatedSSHSecurityGroup created for this one instance
// (CreateOpts.AllowSSHInbound) is a different story: it's tagged as
// dedicated at creation, so cleanupDedicatedSecurityGroups can safely
// delete it once confirmed no other instance references it. No key pair
// is created either way (there is no SSH key input yet), so there is
// nothing to clean up there.
func (a *AWS) DeleteServer(ctx context.Context, id string) error {
	region, instanceID, err := decodeAWSServerID(id)
	if err != nil {
		return fmt.Errorf("provision: aws delete server: %w", err)
	}
	client, err := a.client(ctx, region)
	if err != nil {
		return fmt.Errorf("provision: aws delete server %q: %w", id, err)
	}
	groupIDs := instanceSecurityGroupIDs(ctx, client, instanceID)
	if _, err := client.TerminateInstances(ctx, &ec2.TerminateInstancesInput{InstanceIds: []string{instanceID}}); err != nil {
		return fmt.Errorf("provision: aws terminate instance %q: %w", instanceID, err)
	}
	cleanupDedicatedSecurityGroups(ctx, client, instanceID, groupIDs)
	return nil
}
