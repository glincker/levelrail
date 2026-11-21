// Package cloudtrail reads the CloudTrail event history for an audit feed.
package cloudtrail

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/GLINCKER/levelrail/internal/awsx"
)

// Service is the CloudTrail API (target prefix checked against the botocore model).
var Service = awsx.Service{Name: "cloudtrail", Prefix: "cloudtrail", SigningName: "cloudtrail", TargetPrefix: "com.amazonaws.cloudtrail.v20131101.CloudTrail_20131101"}

// Lookup attribute keys accepted by LookupEvents. The API allows one per call.
const (
	AttrResourceName = "ResourceName"
	AttrResourceType = "ResourceType"
	AttrUsername     = "Username"
	AttrEventName    = "EventName"
)

// Client is the lazy CloudTrail module.
type Client struct{ aws *awsx.Client }

// New wraps a shared client.
func New(c *awsx.Client) *Client { return &Client{aws: c} }

// Event is one audit row. Raw JSON is left out on purpose.
type Event struct {
	ID, Name, Source, Username string
	Time                       time.Time
	Resources                  []string
}

// Query selects events. Zero times mean the API default window.
type Query struct {
	Region     string
	AttrKey    string
	AttrValue  string
	Start, End time.Time
	// MaxPages bounds pagination; below 1 reads one page.
	MaxPages int
}

type lookupAttr struct {
	AttributeKey   string `json:"AttributeKey"`
	AttributeValue string `json:"AttributeValue"`
}

// LookupEvents returns events matching one attribute, newest first.
func (c *Client) LookupEvents(ctx context.Context, q Query) ([]Event, error) {
	if q.AttrKey == "" || q.AttrValue == "" {
		return nil, errors.New("cloudtrail: a lookup attribute is required")
	}
	pages := max(q.MaxPages, 1)
	var res []Event
	token := ""
	for p := 0; p < pages; p++ {
		in := map[string]any{"LookupAttributes": []lookupAttr{{q.AttrKey, q.AttrValue}}}
		if !q.Start.IsZero() {
			in["StartTime"] = q.Start.Unix()
		}
		if !q.End.IsZero() {
			in["EndTime"] = q.End.Unix()
		}
		if token != "" {
			in["NextToken"] = token
		}
		var out struct {
			Events []struct {
				ID        string         `json:"EventId"`
				Name      string         `json:"EventName"`
				Source    string         `json:"EventSource"`
				Username  string         `json:"Username"`
				Time      awsx.EpochTime `json:"EventTime"`
				Resources []struct {
					Name string `json:"ResourceName"`
				} `json:"Resources"`
			} `json:"Events"`
			Next string `json:"NextToken"`
		}
		if err := c.aws.JSON(ctx, Service, q.Region, "LookupEvents", in, &out); err != nil {
			return nil, err
		}
		for _, e := range out.Events {
			ev := Event{ID: e.ID, Name: e.Name, Source: e.Source, Username: e.Username, Time: e.Time.Time}
			for _, r := range e.Resources {
				ev.Resources = append(ev.Resources, r.Name)
			}
			res = append(res, ev)
		}
		if out.Next == "" {
			break
		}
		token = out.Next
	}
	sort.SliceStable(res, func(i, j int) bool { return res[i].Time.After(res[j].Time) })
	return res, nil
}

// LookupByNames merges the feed for several resource names, for a tag query:
// LookupEvents cannot filter by tag, so callers resolve the tag to resource
// names first (the Tagging API sweep in cloudguard) and pass them here.
func (c *Client) LookupByNames(ctx context.Context, q Query, names []string) ([]Event, error) {
	seen := map[string]bool{}
	var res []Event
	for _, n := range names {
		qq := q
		qq.AttrKey, qq.AttrValue = AttrResourceName, n
		evs, err := c.LookupEvents(ctx, qq)
		if err != nil {
			return nil, err
		}
		for _, e := range evs {
			if !seen[e.ID] {
				seen[e.ID] = true
				res = append(res, e)
			}
		}
	}
	sort.SliceStable(res, func(i, j int) bool { return res[i].Time.After(res[j].Time) })
	return res, nil
}
