package cloudguard

import (
	"context"
	"errors"
	"net/url"
	"strconv"

	"github.com/glincker/levelrail/internal/awsx"
)

// ErrRefused wraps every guard refusal of an EC2 create.
var ErrRefused = errors.New("cloudguard: create refused")

// InstanceInput is one EC2 create request.
type InstanceInput struct {
	Account, Region, AMI, Type, Name, SubnetID string
	EstimatedMonthlyUSD                        float64
	Meta                                       TagMeta
}

// Authorize runs the guard for one EC2 create and returns the full tag set to apply.
// A refusal carries the guard code and message so callers can answer with a clear 4xx.
func (g *Guard) Authorize(ctx context.Context, in InstanceInput) (map[string]string, Decision) {
	base := map[string]string{}
	if in.Name != "" {
		base["Name"] = in.Name
	}
	tags := InjectTags(g.cfg(), base, in.Meta)
	d := g.Check(ctx, CreateRequest{Account: in.Account, Service: "ec2", Resource: in.Name,
		InstanceType: in.Type, EstimatedMonthlyUSD: in.EstimatedMonthlyUSD, Tags: tags})
	return tags, d
}

// RunInstance creates one EC2 instance. The guard decides first and the mandatory
// tags go on the RunInstances call itself, so the instance is never untagged.
func RunInstance(ctx context.Context, c *awsx.Client, g *Guard, in InstanceInput) (string, error) {
	tags, d := g.Authorize(ctx, in)
	if !d.Allowed {
		return "", errors.Join(ErrRefused, errors.New(d.Message))
	}
	p := url.Values{"ImageId": {in.AMI}, "InstanceType": {in.Type}, "MinCount": {"1"}, "MaxCount": {"1"},
		"TagSpecification.1.ResourceType": {"instance"}}
	if in.SubnetID != "" {
		p.Set("SubnetId", in.SubnetID)
	}
	for i, t := range awsx.TagList(tags) {
		n := "TagSpecification.1.Tag." + strconv.Itoa(i+1)
		p.Set(n+".Key", t.Key)
		p.Set(n+".Value", t.Value)
	}
	var res struct {
		ID string `xml:"instancesSet>item>instanceId"`
	}
	if err := c.Query(ctx, ec2Svc, in.Region, "RunInstances", ec2Version, p, &res); err != nil {
		return "", err
	}
	return res.ID, nil
}
