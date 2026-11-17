package provision

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// pagedEC2 serves DescribeInstances pages keyed by NextToken.
type pagedEC2 struct {
	*fakeEC2
	pages map[string]*ec2.DescribeInstancesOutput
	calls int
}

func (p *pagedEC2) DescribeInstances(_ context.Context, in *ec2.DescribeInstancesInput, _ ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
	p.calls++
	return p.pages[aws.ToString(in.NextToken)], nil
}

func awsTestInstance(id string, state types.InstanceStateName, name string) types.Instance {
	return types.Instance{
		InstanceId: aws.String(id), InstanceType: types.InstanceTypeT3Micro,
		State:           &types.InstanceState{Name: state},
		PublicIpAddress: aws.String("203.0.113.7"),
		Tags:            []types.Tag{{Key: aws.String("Name"), Value: aws.String(name)}},
	}
}

func TestAWS_ListServers_PaginatesAndFiltersState(t *testing.T) {
	fake := &pagedEC2{fakeEC2: &fakeEC2{}, pages: map[string]*ec2.DescribeInstancesOutput{
		"": {
			NextToken: aws.String("p2"),
			Reservations: []types.Reservation{{Instances: []types.Instance{
				awsTestInstance("i-1", types.InstanceStateNameRunning, "web"),
				awsTestInstance("i-2", types.InstanceStateNameTerminated, "gone"),
			}}},
		},
		"p2": {Reservations: []types.Reservation{{Instances: []types.Instance{
			awsTestInstance("i-3", types.InstanceStateNameStopped, "db"),
			awsTestInstance("i-4", types.InstanceStateNameShuttingDown, "dying"),
		}}}},
	}}
	a := NewAWS(AWSCredentials{AccessKeyID: "ak", SecretAccessKey: "sk", Region: "us-east-1"}, "app")
	a.client = func(context.Context, string) (ec2API, error) { return fake, nil }

	got, err := a.ListServers(context.Background(), "eu-west-1")
	if err != nil {
		t.Fatal(err)
	}
	if fake.calls != 2 {
		t.Fatalf("describe calls = %d, want 2", fake.calls)
	}
	if len(got) != 2 || got[0].ID != "eu-west-1/i-1" || got[0].Name != "web" || got[1].ID != "eu-west-1/i-3" || got[1].Status != "stopped" {
		t.Fatalf("unexpected servers: %+v", got)
	}
}
