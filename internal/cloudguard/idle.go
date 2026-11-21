package cloudguard

import (
	"context"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/awsx"
	"github.com/GLINCKER/levelrail/internal/awsx/cloudwatch"
)

var elbSvc = awsx.Service{Name: "elasticloadbalancing", Prefix: "elasticloadbalancing", SigningName: "elasticloadbalancing"}

// elbVersion is the Query API version of Elastic Load Balancing v2.
const elbVersion = "2015-12-01"

// Idle suggestion kinds and actions.
const (
	KindEC2LowCPU     = "ec2_low_cpu"
	KindEBSUnattached = "ebs_unattached"
	KindEIPFree       = "eip_unassociated"
	KindALBNoRequests = "alb_no_requests"
	KindRDSStopped    = "rds_stopped"

	ActionStop           = "stop"
	ActionSnapshotDelete = "snapshot_delete"
	ActionRelease        = "release"
	ActionDelete         = "delete"
)

// IdleConfig carries the live idle thresholds and the unit prices behind the savings.
type IdleConfig struct {
	CPUPercent    float64
	Days          int
	EBSGBMonthUSD float64
	EIPMonthUSD   float64
	ALBMonthUSD   float64
	Now           func() time.Time
}

// Suggestion is one idle resource. The control plane only reports it; it never acts.
type Suggestion struct {
	Kind, ID, Detail, Action string
	// MonthlyUSD is the estimated saving, zero when no estimate exists.
	MonthlyUSD float64
}

func (c IdleConfig) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

// FindIdle is a dry run. Metrics are read in batches, so the AWS bill for it stays small.
func FindIdle(ctx context.Context, c *awsx.Client, cfg IdleConfig, region string, maxPages int) ([]Suggestion, error) {
	if maxPages < 1 {
		maxPages = 1
	}
	if cfg.Days < 1 {
		cfg.Days = 7
	}
	var out []Suggestion
	cpu, err := idleInstances(ctx, c, cfg, region, maxPages)
	if err != nil {
		return nil, err
	}
	out = append(out, cpu...)
	vols, err := idleVolumes(ctx, c, cfg, region, maxPages)
	if err != nil {
		return nil, err
	}
	out = append(out, vols...)
	var addr ec2Addresses
	if err := c.Query(ctx, ec2Svc, region, "DescribeAddresses", ec2Version, nil, &addr); err != nil {
		return nil, err
	}
	for _, a := range addr.Items {
		if a.Association == "" {
			out = append(out, Suggestion{Kind: KindEIPFree, ID: a.AllocationID, Detail: a.PublicIP, Action: ActionRelease, MonthlyUSD: cfg.EIPMonthUSD})
		}
	}
	albs, err := idleALBs(ctx, c, cfg, region, maxPages)
	if err != nil {
		return nil, err
	}
	out = append(out, albs...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func idleInstances(ctx context.Context, c *awsx.Client, cfg IdleConfig, region string, maxPages int) ([]Suggestion, error) {
	type inst struct{ id, typ string }
	var list []inst
	token := ""
	for page := 0; page < maxPages; page++ {
		p := url.Values{"Filter.1.Name": {"instance-state-name"}, "Filter.1.Value.1": {"running"}, "MaxResults": {"500"}}
		if token != "" {
			p.Set("NextToken", token)
		}
		var res struct {
			NextToken string `xml:"nextToken"`
			Items     []struct {
				ID   string `xml:"instanceId"`
				Type string `xml:"instanceType"`
			} `xml:"reservationSet>item>instancesSet>item"`
		}
		if err := c.Query(ctx, ec2Svc, region, "DescribeInstances", ec2Version, p, &res); err != nil {
			return nil, err
		}
		for _, it := range res.Items {
			list = append(list, inst{it.ID, it.Type})
		}
		if res.NextToken == "" {
			break
		}
		token = res.NextToken
	}
	if len(list) == 0 {
		return nil, nil
	}
	end := cfg.now().UTC()
	start := end.Add(-time.Duration(cfg.Days) * 24 * time.Hour)
	qs := make([]cloudwatch.Query, len(list))
	for i, it := range list {
		qs[i] = cloudwatch.Query{ID: "i" + strconv.Itoa(i), Namespace: "AWS/EC2", Name: "CPUUtilization", Stat: "Average", Period: 86400,
			Dimensions: []cloudwatch.Dimension{{Name: "InstanceId", Value: it.id}}}
	}
	series, err := cloudwatch.New(c).GetMetricData(ctx, region, qs, start, end)
	if err != nil {
		return nil, err
	}
	var out []Suggestion
	for i, s := range series {
		// Fewer datapoints than days means the instance is younger than the window.
		if len(s.Points) < cfg.Days {
			continue
		}
		peak := 0.0
		for _, pt := range s.Points {
			peak = max(peak, pt.V)
		}
		if peak < cfg.CPUPercent {
			out = append(out, Suggestion{Kind: KindEC2LowCPU, ID: list[i].id, Action: ActionStop,
				Detail: list[i].typ + " daily CPU peak " + strconv.FormatFloat(peak, 'f', 1, 64) + "% over " + strconv.Itoa(cfg.Days) + " days"})
		}
	}
	return out, nil
}

func idleVolumes(ctx context.Context, c *awsx.Client, cfg IdleConfig, region string, maxPages int) ([]Suggestion, error) {
	var out []Suggestion
	token := ""
	for page := 0; page < maxPages; page++ {
		p := url.Values{"Filter.1.Name": {"status"}, "Filter.1.Value.1": {"available"}, "MaxResults": {"500"}}
		if token != "" {
			p.Set("NextToken", token)
		}
		var res ec2Volumes
		if err := c.Query(ctx, ec2Svc, region, "DescribeVolumes", ec2Version, p, &res); err != nil {
			return nil, err
		}
		for _, v := range res.Items {
			out = append(out, Suggestion{Kind: KindEBSUnattached, ID: v.ID, Action: ActionSnapshotDelete,
				Detail: strconv.Itoa(v.Size) + " GiB " + v.Type, MonthlyUSD: float64(v.Size) * cfg.EBSGBMonthUSD})
		}
		if res.NextToken == "" {
			break
		}
		token = res.NextToken
	}
	return out, nil
}

func idleALBs(ctx context.Context, c *awsx.Client, cfg IdleConfig, region string, maxPages int) ([]Suggestion, error) {
	type lb struct{ name, dim string }
	var list []lb
	end := cfg.now().UTC()
	start := end.Add(-time.Duration(cfg.Days) * 24 * time.Hour)
	marker := ""
	for page := 0; page < maxPages; page++ {
		p := url.Values{"PageSize": {"400"}}
		if marker != "" {
			p.Set("Marker", marker)
		}
		var res struct {
			Next string `xml:"DescribeLoadBalancersResult>NextMarker"`
			LBs  []struct {
				ARN     string `xml:"LoadBalancerArn"`
				Name    string `xml:"LoadBalancerName"`
				Type    string `xml:"Type"`
				Created string `xml:"CreatedTime"`
			} `xml:"DescribeLoadBalancersResult>LoadBalancers>member"`
		}
		if err := c.Query(ctx, elbSvc, region, "DescribeLoadBalancers", elbVersion, p, &res); err != nil {
			return nil, err
		}
		for _, l := range res.LBs {
			created, _ := time.Parse(time.RFC3339, strings.TrimSpace(l.Created))
			_, dim, ok := strings.Cut(l.ARN, "loadbalancer/")
			// A balancer younger than the window has not had time to be idle.
			if l.Type == "application" && ok && !created.IsZero() && created.Before(start) {
				list = append(list, lb{l.Name, dim})
			}
		}
		if res.Next == "" {
			break
		}
		marker = res.Next
	}
	if len(list) == 0 {
		return nil, nil
	}
	qs := make([]cloudwatch.Query, len(list))
	for i, l := range list {
		qs[i] = cloudwatch.Query{ID: "l" + strconv.Itoa(i), Namespace: "AWS/ApplicationELB", Name: "RequestCount", Stat: "Sum", Period: 86400,
			Dimensions: []cloudwatch.Dimension{{Name: "LoadBalancer", Value: l.dim}}}
	}
	series, err := cloudwatch.New(c).GetMetricData(ctx, region, qs, start, end)
	if err != nil {
		return nil, err
	}
	var out []Suggestion
	for i, s := range series {
		// RequestCount is not published when it is zero, so no points also means idle.
		total := 0.0
		for _, pt := range s.Points {
			total += pt.V
		}
		if total == 0 {
			out = append(out, Suggestion{Kind: KindALBNoRequests, ID: list[i].name, Action: ActionDelete,
				Detail: "no requests in " + strconv.Itoa(cfg.Days) + " days", MonthlyUSD: cfg.ALBMonthUSD})
		}
	}
	return out, nil
}
