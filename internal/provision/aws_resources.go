package provision

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// ListResources lists the instances in the credentials' default region so
// the shared discovery endpoint works for AWS. Read-only.
func (a *AWS) ListResources(ctx context.Context) ([]ProviderResource, error) {
	return a.ListServers(ctx, "")
}

// ListServers pages through DescribeInstances for region (the credentials'
// default when empty) and keeps running and stopped instances only, so
// terminated ones never show up as import candidates. IDs use the same
// region/instance form GetServer decodes, so enrollment matching lines up.
func (a *AWS) ListServers(ctx context.Context, region string) ([]ProviderResource, error) {
	if region == "" {
		region = a.creds.region()
	}
	client, err := a.client(ctx, region)
	if err != nil {
		return nil, fmt.Errorf("provision: aws list servers: %w", err)
	}
	in := &ec2.DescribeInstancesInput{Filters: []types.Filter{{
		Name:   aws.String("instance-state-name"),
		Values: []string{string(types.InstanceStateNameRunning), string(types.InstanceStateNameStopped)},
	}}}
	res := []ProviderResource{}
	for pages := 0; pages < maxListPages; pages++ {
		out, err := client.DescribeInstances(ctx, in)
		if err != nil {
			return nil, fmt.Errorf("provision: aws list servers: %w", err)
		}
		for _, r := range out.Reservations {
			for _, inst := range r.Instances {
				if pr, ok := awsResource(region, inst); ok {
					res = append(res, pr)
				}
			}
		}
		if out.NextToken == nil || *out.NextToken == "" {
			break
		}
		in.NextToken = out.NextToken
	}
	return res, nil
}

// awsResource re-checks the state client-side because some API shims ignore
// the filter. The Name tag becomes the display name, other tags are kept.
func awsResource(region string, inst types.Instance) (ProviderResource, bool) {
	if inst.State == nil || (inst.State.Name != types.InstanceStateNameRunning && inst.State.Name != types.InstanceStateNameStopped) {
		return ProviderResource{}, false
	}
	pr := ProviderResource{
		ID:         encodeAWSServerID(region, aws.ToString(inst.InstanceId)),
		Region:     region,
		Size:       string(inst.InstanceType),
		Status:     string(inst.State.Name),
		PublicIPv4: aws.ToString(inst.PublicIpAddress),
		Tags:       []string{},
	}
	if inst.LaunchTime != nil {
		pr.CreatedAt = inst.LaunchTime.UTC()
	}
	for _, t := range inst.Tags {
		k, v := aws.ToString(t.Key), aws.ToString(t.Value)
		if k == "Name" {
			pr.Name = v
			continue
		}
		pr.Tags = append(pr.Tags, k+"="+v)
	}
	return pr, true
}
