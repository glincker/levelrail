package provision

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// fakeEC2 is a hand-written fake for ec2API, the same "narrow interface
// plus a hand-written fake" pattern internal/api's fakeNodeProvisioner
// already establishes for NodeProvisioner: no network I/O, no XML wire
// format to emulate.
type fakeEC2 struct {
	regions []types.Region

	images         []types.Image
	vpcs           []types.Vpc
	subnets        []types.Subnet
	securityGroups []types.SecurityGroup

	createSGErr error
	createdSG   string

	authorizeIngressErr error

	deleteSGErr error

	runOut *ec2.RunInstancesOutput
	runErr error

	describeOut *ec2.DescribeInstancesOutput
	describeErr error

	terminateErr error

	authErr error // when set, every call fails with this (simulates bad credentials)

	runCalls              int
	describeCalls         int
	terminateCalls        int
	createSGCalls         int
	authorizeIngressCalls int
	deleteSGCalls         int
}

func (f *fakeEC2) DescribeRegions(context.Context, *ec2.DescribeRegionsInput, ...func(*ec2.Options)) (*ec2.DescribeRegionsOutput, error) {
	if f.authErr != nil {
		return nil, f.authErr
	}
	return &ec2.DescribeRegionsOutput{Regions: f.regions}, nil
}

func (f *fakeEC2) DescribeImages(context.Context, *ec2.DescribeImagesInput, ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error) {
	if f.authErr != nil {
		return nil, f.authErr
	}
	return &ec2.DescribeImagesOutput{Images: f.images}, nil
}

func (f *fakeEC2) DescribeVpcs(context.Context, *ec2.DescribeVpcsInput, ...func(*ec2.Options)) (*ec2.DescribeVpcsOutput, error) {
	if f.authErr != nil {
		return nil, f.authErr
	}
	return &ec2.DescribeVpcsOutput{Vpcs: f.vpcs}, nil
}

func (f *fakeEC2) DescribeSubnets(context.Context, *ec2.DescribeSubnetsInput, ...func(*ec2.Options)) (*ec2.DescribeSubnetsOutput, error) {
	if f.authErr != nil {
		return nil, f.authErr
	}
	return &ec2.DescribeSubnetsOutput{Subnets: f.subnets}, nil
}

func (f *fakeEC2) DescribeSecurityGroups(context.Context, *ec2.DescribeSecurityGroupsInput, ...func(*ec2.Options)) (*ec2.DescribeSecurityGroupsOutput, error) {
	if f.authErr != nil {
		return nil, f.authErr
	}
	return &ec2.DescribeSecurityGroupsOutput{SecurityGroups: f.securityGroups}, nil
}

func (f *fakeEC2) CreateSecurityGroup(_ context.Context, in *ec2.CreateSecurityGroupInput, _ ...func(*ec2.Options)) (*ec2.CreateSecurityGroupOutput, error) {
	f.createSGCalls++
	if f.authErr != nil {
		return nil, f.authErr
	}
	if f.createSGErr != nil {
		return nil, f.createSGErr
	}
	id := f.createdSG
	if id == "" {
		id = "sg-created"
	}
	_ = in
	return &ec2.CreateSecurityGroupOutput{GroupId: aws.String(id)}, nil
}

func (f *fakeEC2) AuthorizeSecurityGroupIngress(context.Context, *ec2.AuthorizeSecurityGroupIngressInput, ...func(*ec2.Options)) (*ec2.AuthorizeSecurityGroupIngressOutput, error) {
	f.authorizeIngressCalls++
	if f.authErr != nil {
		return nil, f.authErr
	}
	if f.authorizeIngressErr != nil {
		return nil, f.authorizeIngressErr
	}
	return &ec2.AuthorizeSecurityGroupIngressOutput{}, nil
}

func (f *fakeEC2) DeleteSecurityGroup(context.Context, *ec2.DeleteSecurityGroupInput, ...func(*ec2.Options)) (*ec2.DeleteSecurityGroupOutput, error) {
	f.deleteSGCalls++
	if f.authErr != nil {
		return nil, f.authErr
	}
	if f.deleteSGErr != nil {
		return nil, f.deleteSGErr
	}
	return &ec2.DeleteSecurityGroupOutput{}, nil
}

func (f *fakeEC2) RunInstances(context.Context, *ec2.RunInstancesInput, ...func(*ec2.Options)) (*ec2.RunInstancesOutput, error) {
	f.runCalls++
	if f.authErr != nil {
		return nil, f.authErr
	}
	if f.runErr != nil {
		return nil, f.runErr
	}
	return f.runOut, nil
}

func (f *fakeEC2) DescribeInstances(context.Context, *ec2.DescribeInstancesInput, ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
	f.describeCalls++
	if f.authErr != nil {
		return nil, f.authErr
	}
	if f.describeErr != nil {
		return nil, f.describeErr
	}
	return f.describeOut, nil
}

func (f *fakeEC2) TerminateInstances(context.Context, *ec2.TerminateInstancesInput, ...func(*ec2.Options)) (*ec2.TerminateInstancesOutput, error) {
	f.terminateCalls++
	if f.authErr != nil {
		return nil, f.authErr
	}
	if f.terminateErr != nil {
		return nil, f.terminateErr
	}
	return &ec2.TerminateInstancesOutput{}, nil
}

func marshalAWSCreds(c AWSCredentials) (string, error) {
	b, err := json.Marshal(c) //nolint:gosec // test helper only, encodes fixture values, never logged
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func newTestAWS(fake *fakeEC2) *AWS {
	a := NewAWS(AWSCredentials{AccessKeyID: "ak", SecretAccessKey: "sk", Region: "us-east-1"}, "app")
	a.client = func(context.Context, string) (ec2API, error) { return fake, nil }
	return a
}

func defaultCreateFixture() *fakeEC2 {
	return &fakeEC2{
		images: []types.Image{{
			ImageId: aws.String("ami-123"), RootDeviceName: aws.String("/dev/sda1"),
			CreationDate: aws.String("2026-01-01T00:00:00.000Z"),
		}},
		vpcs:    []types.Vpc{{VpcId: aws.String("vpc-1")}},
		subnets: []types.Subnet{{SubnetId: aws.String("subnet-1")}},
		runOut: &ec2.RunInstancesOutput{Instances: []types.Instance{{
			InstanceId: aws.String("i-abc123"), PublicIpAddress: aws.String("1.2.3.4"),
		}}},
	}
}

func TestAWS_NewAWSFromToken_RoundTrips(t *testing.T) {
	creds := AWSCredentials{AccessKeyID: "ak", SecretAccessKey: "sk", Region: "eu-west-1"}
	raw, err := marshalAWSCreds(creds)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	a, err := NewAWSFromToken(raw, "app")
	if err != nil {
		t.Fatalf("NewAWSFromToken: %v", err)
	}
	if a.creds.AccessKeyID != "ak" || a.creds.Region != "eu-west-1" {
		t.Errorf("creds = %+v", a.creds)
	}
}

func TestAWS_NewAWSFromToken_MissingSecretFails(t *testing.T) {
	raw, err := marshalAWSCreds(AWSCredentials{AccessKeyID: "ak"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if _, err := NewAWSFromToken(raw, "app"); err == nil {
		t.Error("NewAWSFromToken: want error for missing secret access key")
	}
}

func TestAWS_NewAWSFromToken_AmbientCredentialsSkipsKeyCheck(t *testing.T) {
	raw, err := marshalAWSCreds(AWSCredentials{UseAmbientCredentials: true})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if _, err := NewAWSFromToken(raw, "app"); err != nil {
		t.Errorf("NewAWSFromToken: %v", err)
	}
}

func TestAWS_NewAWSFromToken_BadJSON(t *testing.T) {
	if _, err := NewAWSFromToken("not json", "app"); err == nil {
		t.Error("NewAWSFromToken: want error for malformed json")
	}
}

func TestAWS_ListRegions(t *testing.T) {
	fake := &fakeEC2{regions: []types.Region{
		{RegionName: aws.String("us-east-1")},
		{RegionName: aws.String("ap-south-1")},
	}}
	a := newTestAWS(fake)
	regions, err := a.ListRegions(context.Background())
	if err != nil {
		t.Fatalf("ListRegions: %v", err)
	}
	if len(regions) != 2 || regions[0].ID != "us-east-1" || regions[0].Name == "us-east-1" {
		t.Errorf("regions = %+v, want a friendly name for us-east-1", regions)
	}
}

func TestAWS_ListRegions_AuthFailure(t *testing.T) {
	fake := &fakeEC2{authErr: errors.New("AuthFailure: credentials invalid")}
	a := newTestAWS(fake)
	if _, err := a.ListRegions(context.Background()); err == nil {
		t.Error("ListRegions: want error on auth failure")
	}
}

func TestAWS_ListSizes_ReturnsCuratedStaticList(t *testing.T) {
	a := newTestAWS(&fakeEC2{})
	sizes, err := a.ListSizes(context.Background(), "us-east-1")
	if err != nil {
		t.Fatalf("ListSizes: %v", err)
	}
	if len(sizes) == 0 {
		t.Fatal("ListSizes: want a non-empty curated list")
	}
	for _, s := range sizes {
		if s.Disk != awsDefaultDiskGB {
			t.Errorf("size %s Disk = %d, want %d", s.ID, s.Disk, awsDefaultDiskGB)
		}
	}
	// Mutating the returned slice must not affect the next call: it's a
	// defensive copy of the package-level curated list.
	sizes[0].ID = "mutated"
	again, err := a.ListSizes(context.Background(), "us-east-1")
	if err != nil {
		t.Fatalf("ListSizes: %v", err)
	}
	if again[0].ID == "mutated" {
		t.Error("ListSizes leaked its backing array across calls")
	}
}

func TestAWS_CreateServer_Success(t *testing.T) {
	fake := defaultCreateFixture()
	a := newTestAWS(fake)
	id, ip, err := a.CreateServer(context.Background(), CreateOpts{
		Region: "us-east-1", Size: "t3.small", Name: "web-1", UserData: "#cloud-init",
	})
	if err != nil {
		t.Fatalf("CreateServer: %v", err)
	}
	if id != "us-east-1/i-abc123" {
		t.Errorf("id = %q, want region-prefixed instance id", id)
	}
	if ip != "1.2.3.4" {
		t.Errorf("ip = %q, want 1.2.3.4", ip)
	}
	if fake.runCalls != 1 {
		t.Errorf("runCalls = %d, want 1", fake.runCalls)
	}
}

func TestAWS_CreateServer_CreatesSecurityGroupOnceThenReuses(t *testing.T) {
	fake := defaultCreateFixture()
	fake.createdSG = "sg-new"
	a := newTestAWS(fake)

	if _, _, err := a.CreateServer(context.Background(), CreateOpts{Region: "us-east-1", Size: "t3.small", Name: "web-1", UserData: "x"}); err != nil {
		t.Fatalf("CreateServer 1: %v", err)
	}
	// Second call: DescribeSecurityGroups now finds the group the first
	// call created, so CreateSecurityGroup must not run again.
	fake.securityGroups = []types.SecurityGroup{{GroupId: aws.String("sg-new")}}
	fake.createSGErr = errors.New("CreateSecurityGroup should not be called again")
	if _, _, err := a.CreateServer(context.Background(), CreateOpts{Region: "us-east-1", Size: "t3.small", Name: "web-2", UserData: "x"}); err != nil {
		t.Fatalf("CreateServer 2: %v", err)
	}
}

func TestAWS_CreateServer_MissingRegion(t *testing.T) {
	a := newTestAWS(defaultCreateFixture())
	if _, _, err := a.CreateServer(context.Background(), CreateOpts{Size: "t3.small", Name: "web-1", UserData: "x"}); err == nil {
		t.Error("CreateServer: want error when region is empty")
	}
}

func TestAWS_CreateServer_NoDefaultVPC(t *testing.T) {
	fake := defaultCreateFixture()
	fake.vpcs = nil
	a := newTestAWS(fake)
	if _, _, err := a.CreateServer(context.Background(), CreateOpts{Region: "us-east-1", Size: "t3.small", Name: "web-1", UserData: "x"}); err == nil {
		t.Error("CreateServer: want error when no default vpc exists")
	}
}

func TestAWS_CreateServer_NoUbuntuImageFound(t *testing.T) {
	fake := defaultCreateFixture()
	fake.images = nil
	a := newTestAWS(fake)
	if _, _, err := a.CreateServer(context.Background(), CreateOpts{Region: "us-east-1", Size: "t3.small", Name: "web-1", UserData: "x"}); err == nil {
		t.Error("CreateServer: want error when no ubuntu ami is found")
	}
}

func TestAWS_CreateServer_AuthFailure(t *testing.T) {
	fake := defaultCreateFixture()
	fake.authErr = errors.New("AuthFailure")
	a := newTestAWS(fake)
	if _, _, err := a.CreateServer(context.Background(), CreateOpts{Region: "us-east-1", Size: "t3.small", Name: "web-1", UserData: "x"}); err == nil {
		t.Error("CreateServer: want error on auth failure")
	}
}

func TestAWS_GetServer(t *testing.T) {
	tests := []struct {
		name       string
		state      types.InstanceStateName
		wantStatus ServerStatus
	}{
		{"running", types.InstanceStateNameRunning, ServerStatusRunning},
		{"pending", types.InstanceStateNamePending, ServerStatusPending},
		{"terminated", types.InstanceStateNameTerminated, ServerStatusError},
		{"stopped", types.InstanceStateNameStopped, ServerStatusError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeEC2{describeOut: &ec2.DescribeInstancesOutput{
				Reservations: []types.Reservation{{Instances: []types.Instance{{
					InstanceId: aws.String("i-abc123"), PublicIpAddress: aws.String("1.2.3.4"),
					State: &types.InstanceState{Name: tt.state},
				}}}},
			}}
			a := newTestAWS(fake)
			status, ip, err := a.GetServer(context.Background(), "us-east-1/i-abc123")
			if err != nil {
				t.Fatalf("GetServer: %v", err)
			}
			if status != tt.wantStatus {
				t.Errorf("status = %q, want %q", status, tt.wantStatus)
			}
			if ip != "1.2.3.4" {
				t.Errorf("ip = %q, want 1.2.3.4", ip)
			}
		})
	}
}

func TestAWS_GetServer_NotFound(t *testing.T) {
	fake := &fakeEC2{describeOut: &ec2.DescribeInstancesOutput{}}
	a := newTestAWS(fake)
	if _, _, err := a.GetServer(context.Background(), "us-east-1/i-missing"); err == nil {
		t.Error("GetServer: want error when the instance isn't found")
	}
}

func TestAWS_GetServer_MalformedID(t *testing.T) {
	a := newTestAWS(&fakeEC2{})
	if _, _, err := a.GetServer(context.Background(), "i-abc123"); err == nil {
		t.Error("GetServer: want error for an id with no region prefix")
	}
}

func TestAWS_GetServer_AuthFailure(t *testing.T) {
	fake := &fakeEC2{authErr: errors.New("AuthFailure")}
	a := newTestAWS(fake)
	if _, _, err := a.GetServer(context.Background(), "us-east-1/i-abc123"); err == nil {
		t.Error("GetServer: want error on auth failure")
	}
}

func TestAWS_DeleteServer(t *testing.T) {
	fake := &fakeEC2{}
	a := newTestAWS(fake)
	if err := a.DeleteServer(context.Background(), "us-east-1/i-abc123"); err != nil {
		t.Fatalf("DeleteServer: %v", err)
	}
	if fake.terminateCalls != 1 {
		t.Errorf("terminateCalls = %d, want 1", fake.terminateCalls)
	}
}

func TestAWS_DeleteServer_MalformedID(t *testing.T) {
	a := newTestAWS(&fakeEC2{})
	if err := a.DeleteServer(context.Background(), "i-abc123"); err == nil {
		t.Error("DeleteServer: want error for an id with no region prefix")
	}
}

func TestAWS_DeleteServer_ProviderError(t *testing.T) {
	fake := &fakeEC2{terminateErr: errors.New("InvalidInstanceID.NotFound")}
	a := newTestAWS(fake)
	if err := a.DeleteServer(context.Background(), "us-east-1/i-abc123"); err == nil {
		t.Error("DeleteServer: want error when termination fails")
	}
}

func TestAWS_CreateServer_SSHInboundUsesDedicatedGroup(t *testing.T) {
	fake := defaultCreateFixture()
	fake.createdSG = "sg-ssh"
	a := newTestAWS(fake)

	if _, _, err := a.CreateServer(context.Background(), CreateOpts{
		Region: "us-east-1", Size: "t3.small", Name: "web-1", UserData: "x", AllowSSHInbound: true,
	}); err != nil {
		t.Fatalf("CreateServer: %v", err)
	}
	if fake.createSGCalls != 1 {
		t.Errorf("createSGCalls = %d, want 1", fake.createSGCalls)
	}
	if fake.authorizeIngressCalls != 1 {
		t.Errorf("authorizeIngressCalls = %d, want 1 (ssh ingress rule)", fake.authorizeIngressCalls)
	}
}

func TestAWS_CreateServer_NoSSHInboundSkipsAuthorize(t *testing.T) {
	fake := defaultCreateFixture()
	a := newTestAWS(fake)

	if _, _, err := a.CreateServer(context.Background(), CreateOpts{
		Region: "us-east-1", Size: "t3.small", Name: "web-1", UserData: "x",
	}); err != nil {
		t.Fatalf("CreateServer: %v", err)
	}
	if fake.authorizeIngressCalls != 0 {
		t.Errorf("authorizeIngressCalls = %d, want 0 for a default (no ssh) create", fake.authorizeIngressCalls)
	}
}

func TestAWS_DeleteServer_DeletesDedicatedGroupWhenUnreferenced(t *testing.T) {
	fake := &fakeEC2{
		describeOut: &ec2.DescribeInstancesOutput{
			Reservations: []types.Reservation{{Instances: []types.Instance{{
				InstanceId:     aws.String("i-abc123"),
				SecurityGroups: []types.GroupIdentifier{{GroupId: aws.String("sg-ssh")}},
			}}}},
		},
		securityGroups: []types.SecurityGroup{{
			GroupId: aws.String("sg-ssh"),
			Tags:    []types.Tag{{Key: aws.String(awsDedicatedSecurityGroupTagKey), Value: aws.String("true")}},
		}},
	}
	a := newTestAWS(fake)
	if err := a.DeleteServer(context.Background(), "us-east-1/i-abc123"); err != nil {
		t.Fatalf("DeleteServer: %v", err)
	}
	if fake.deleteSGCalls != 1 {
		t.Errorf("deleteSGCalls = %d, want 1", fake.deleteSGCalls)
	}
}

func TestAWS_DeleteServer_KeepsDedicatedGroupStillReferenced(t *testing.T) {
	fake := &fakeEC2{
		describeOut: &ec2.DescribeInstancesOutput{
			Reservations: []types.Reservation{{Instances: []types.Instance{
				{InstanceId: aws.String("i-abc123"), SecurityGroups: []types.GroupIdentifier{{GroupId: aws.String("sg-ssh")}}},
				{InstanceId: aws.String("i-other"), SecurityGroups: []types.GroupIdentifier{{GroupId: aws.String("sg-ssh")}}},
			}}},
		},
		securityGroups: []types.SecurityGroup{{
			GroupId: aws.String("sg-ssh"),
			Tags:    []types.Tag{{Key: aws.String(awsDedicatedSecurityGroupTagKey), Value: aws.String("true")}},
		}},
	}
	a := newTestAWS(fake)
	if err := a.DeleteServer(context.Background(), "us-east-1/i-abc123"); err != nil {
		t.Fatalf("DeleteServer: %v", err)
	}
	if fake.deleteSGCalls != 0 {
		t.Errorf("deleteSGCalls = %d, want 0: sg-ssh is still referenced by i-other", fake.deleteSGCalls)
	}
}

func TestAWS_DeleteServer_NeverDeletesSharedGroup(t *testing.T) {
	fake := &fakeEC2{
		describeOut: &ec2.DescribeInstancesOutput{
			Reservations: []types.Reservation{{Instances: []types.Instance{{
				InstanceId:     aws.String("i-abc123"),
				SecurityGroups: []types.GroupIdentifier{{GroupId: aws.String("sg-shared")}},
			}}}},
		},
		securityGroups: []types.SecurityGroup{{
			GroupId: aws.String("sg-shared"),
			Tags:    []types.Tag{{Key: aws.String(awsManagedTagKey), Value: aws.String("app")}},
		}},
	}
	a := newTestAWS(fake)
	if err := a.DeleteServer(context.Background(), "us-east-1/i-abc123"); err != nil {
		t.Fatalf("DeleteServer: %v", err)
	}
	if fake.deleteSGCalls != 0 {
		t.Errorf("deleteSGCalls = %d, want 0: sg-shared has no dedicated tag", fake.deleteSGCalls)
	}
}

func TestAWSSecurityGroupName(t *testing.T) {
	tests := []struct{ shortName, want string }{
		{"Acme", "acme-node"},
		{"", "app-node"},
		{"a-b_c", "a-bc-node"},
	}
	for _, tt := range tests {
		if got := awsSecurityGroupName(tt.shortName); got != tt.want {
			t.Errorf("awsSecurityGroupName(%q) = %q, want %q", tt.shortName, got, tt.want)
		}
	}
}

func TestAWSDedicatedSecurityGroupName(t *testing.T) {
	tests := []struct{ shortName, instanceName, want string }{
		{"Acme", "web-1", "acme-web-1-ssh"},
		{"", "web-1", "app-web-1-ssh"},
		{"Acme", "", "acme-instance-ssh"},
	}
	for _, tt := range tests {
		if got := awsDedicatedSecurityGroupName(tt.shortName, tt.instanceName); got != tt.want {
			t.Errorf("awsDedicatedSecurityGroupName(%q, %q) = %q, want %q", tt.shortName, tt.instanceName, got, tt.want)
		}
	}
}

func TestEncodeDecodeAWSServerID(t *testing.T) {
	id := encodeAWSServerID("us-east-1", "i-abc123")
	region, instanceID, err := decodeAWSServerID(id)
	if err != nil {
		t.Fatalf("decodeAWSServerID: %v", err)
	}
	if region != "us-east-1" || instanceID != "i-abc123" {
		t.Errorf("region=%q instanceID=%q", region, instanceID)
	}
	if _, _, err := decodeAWSServerID("no-slash-here"); err == nil {
		t.Error("decodeAWSServerID: want error for a malformed id")
	}
}
