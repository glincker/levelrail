package cloudguard

import (
	"context"
	"net/url"
	"sort"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/awsx"
)

var (
	taggingSvc = awsx.Service{Name: "tagging", Prefix: "tagging", SigningName: "tagging", TargetPrefix: "ResourceGroupsTaggingAPI_20170126"}
	ec2Svc     = awsx.Service{Name: "ec2", Prefix: "ec2", SigningName: "ec2"}
)

// ec2Version is the Query API version of the EC2 describe calls below.
const ec2Version = "2016-11-15"

// Expired is one managed resource past its expires tag.
type Expired struct {
	ARN       string
	ExpiredAt time.Time
}

// Plan is what a sweep would delete. It is always a dry run.
type Plan struct {
	DryRun bool
	Items  []Expired
	// Unparsed lists managed ARNs whose expires tag was not a valid time.
	Unparsed []string
}

type getResourcesIn struct {
	TagFilters       []tagFilter `json:"TagFilters"`
	PaginationToken  string      `json:"PaginationToken,omitempty"`
	ResourcesPerPage int         `json:"ResourcesPerPage,omitempty"`
}

type tagFilter struct {
	Key    string   `json:"Key"`
	Values []string `json:"Values,omitempty"`
}

type getResourcesOut struct {
	PaginationToken     string `json:"PaginationToken"`
	ResourceTagMappings []struct {
		ResourceARN string     `json:"ResourceARN"`
		Tags        []awsx.Tag `json:"Tags"`
	} `json:"ResourceTagMappingList"`
}

// PlanExpired lists managed resources whose expires tag is before now. The
// Tagging API cannot filter by date, so the comparison happens here.
func PlanExpired(ctx context.Context, c *awsx.Client, cfg Config, region string, maxPages int) (Plan, error) {
	if maxPages < 1 {
		maxPages = 1
	}
	now := time.Now().UTC()
	if cfg.Now != nil {
		now = cfg.Now().UTC()
	}
	plan := Plan{DryRun: true}
	expKey := TagKey(cfg, tagExpires)
	token := ""
	for page := 0; page < maxPages; page++ {
		var out getResourcesOut
		in := getResourcesIn{
			TagFilters:      []tagFilter{{Key: TagKey(cfg, tagManaged), Values: []string{"true"}}, {Key: expKey}},
			PaginationToken: token,
		}
		if err := c.JSON(ctx, taggingSvc, region, "GetResources", in, &out); err != nil {
			return Plan{}, err
		}
		for _, m := range out.ResourceTagMappings {
			for _, t := range m.Tags {
				if t.Key != expKey {
					continue
				}
				at, err := time.Parse(time.RFC3339, t.Value)
				switch {
				case err != nil:
					plan.Unparsed = append(plan.Unparsed, m.ResourceARN)
				case at.Before(now):
					plan.Items = append(plan.Items, Expired{ARN: m.ResourceARN, ExpiredAt: at.UTC()})
				}
			}
		}
		if out.PaginationToken == "" {
			break
		}
		token = out.PaginationToken
	}
	sort.Slice(plan.Items, func(i, j int) bool { return plan.Items[i].ARN < plan.Items[j].ARN })
	return plan, nil
}

// Orphan is one resource that looks forgotten.
type Orphan struct {
	ID     string
	Detail string
}

// OrphanReport lists candidates only. Nothing here is ever deleted.
type OrphanReport struct {
	DryRun    bool
	Volumes   []Orphan
	Addresses []Orphan
	Snapshots []Orphan
}

type ec2Volumes struct {
	NextToken string `xml:"nextToken"`
	Items     []struct {
		ID   string `xml:"volumeId"`
		Size int    `xml:"size"`
		Type string `xml:"volumeType"`
	} `xml:"volumeSet>item"`
}

type ec2Addresses struct {
	Items []struct {
		PublicIP     string `xml:"publicIp"`
		AllocationID string `xml:"allocationId"`
		Association  string `xml:"associationId"`
	} `xml:"addressesSet>item"`
}

type ec2Snapshots struct {
	NextToken string `xml:"nextToken"`
	Items     []struct {
		ID       string `xml:"snapshotId"`
		VolumeID string `xml:"volumeId"`
		Size     int    `xml:"volumeSize"`
	} `xml:"snapshotSet>item"`
}

// FindOrphans reads unattached volumes, unassociated Elastic IPs and
// snapshots whose source volume is gone. The status-available filter and the
// self owner filter follow the EC2 DescribeVolumes and DescribeSnapshots docs.
func FindOrphans(ctx context.Context, c *awsx.Client, region string, maxPages int) (OrphanReport, error) {
	if maxPages < 1 {
		maxPages = 1
	}
	rep := OrphanReport{DryRun: true}
	existing := map[string]bool{}
	for _, status := range []string{"", "available"} {
		token := ""
		for page := 0; page < maxPages; page++ {
			p := url.Values{"MaxResults": {strconv.Itoa(500)}}
			if status != "" {
				p.Set("Filter.1.Name", "status")
				p.Set("Filter.1.Value.1", status)
			}
			if token != "" {
				p.Set("NextToken", token)
			}
			var out ec2Volumes
			if err := c.Query(ctx, ec2Svc, region, "DescribeVolumes", ec2Version, p, &out); err != nil {
				return OrphanReport{}, err
			}
			for _, v := range out.Items {
				if status == "" {
					existing[v.ID] = true
					continue
				}
				rep.Volumes = append(rep.Volumes, Orphan{ID: v.ID, Detail: strconv.Itoa(v.Size) + " GiB " + v.Type})
			}
			if out.NextToken == "" {
				break
			}
			token = out.NextToken
		}
	}
	var addr ec2Addresses
	if err := c.Query(ctx, ec2Svc, region, "DescribeAddresses", ec2Version, nil, &addr); err != nil {
		return OrphanReport{}, err
	}
	for _, a := range addr.Items {
		if a.Association == "" {
			rep.Addresses = append(rep.Addresses, Orphan{ID: a.AllocationID, Detail: a.PublicIP})
		}
	}
	token := ""
	for page := 0; page < maxPages; page++ {
		p := url.Values{"Owner.1": {"self"}, "MaxResults": {strconv.Itoa(500)}}
		if token != "" {
			p.Set("NextToken", token)
		}
		var out ec2Snapshots
		if err := c.Query(ctx, ec2Svc, region, "DescribeSnapshots", ec2Version, p, &out); err != nil {
			return OrphanReport{}, err
		}
		for _, s := range out.Items {
			if !existing[s.VolumeID] {
				rep.Snapshots = append(rep.Snapshots, Orphan{ID: s.ID, Detail: s.VolumeID})
			}
		}
		if out.NextToken == "" {
			break
		}
		token = out.NextToken
	}
	return rep, nil
}
