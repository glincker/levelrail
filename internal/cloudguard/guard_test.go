package cloudguard

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/awsx"
	"github.com/GLINCKER/levelrail/internal/awsx/awsxtest"
)

type fakeSpend struct {
	v   float64
	err error
}

func (f fakeSpend) MonthToDateUSD(context.Context, string) (float64, error) { return f.v, f.err }

var now = time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)

func baseCfg() Config {
	return Config{
		MonthlyCapUSD: 100, PerCreateMaxUSD: 20, InstanceTypes: []string{"t3.micro"},
		ServicesEnabled: map[string]bool{"ecr": true, "ec2": true}, DefaultTTL: 24 * time.Hour, Now: func() time.Time { return now },
	}
}

func tagged(cfg Config) map[string]string {
	return InjectTags(cfg, nil, TagMeta{Owner: "o", Env: "e", Project: "p", ID: "i"})
}

func TestCheckTable(t *testing.T) {
	cfg := baseCfg()
	cases := []struct {
		name  string
		mod   func(*Config)
		req   CreateRequest
		spend fakeSpend
		want  string
	}{
		{"allow", nil, CreateRequest{Account: "a", Service: "ecr", EstimatedMonthlyUSD: 5, Tags: tagged(cfg)}, fakeSpend{v: 10}, ""},
		{"kill switch", func(c *Config) { c.KillSwitch = true }, CreateRequest{Account: "a", Service: "ecr", Tags: tagged(cfg)}, fakeSpend{}, CodeKillSwitch},
		{"service off", nil, CreateRequest{Account: "a", Service: "acm", Tags: tagged(cfg)}, fakeSpend{}, CodeServiceDisabled},
		{"per create max", nil, CreateRequest{Account: "a", Service: "ecr", EstimatedMonthlyUSD: 21, Tags: tagged(cfg)}, fakeSpend{}, CodePerCreateMax},
		{"monthly cap", nil, CreateRequest{Account: "a", Service: "ecr", EstimatedMonthlyUSD: 15, Tags: tagged(cfg)}, fakeSpend{v: 90}, CodeMonthlyCap},
		{"account cap override", func(c *Config) { c.AccountCaps = map[string]float64{"a": 12} }, CreateRequest{Account: "a", Service: "ecr", EstimatedMonthlyUSD: 5, Tags: tagged(cfg)}, fakeSpend{v: 10}, CodeMonthlyCap},
		{"spend unknown", nil, CreateRequest{Account: "a", Service: "ecr", Tags: tagged(cfg)}, fakeSpend{err: errors.New("x")}, CodeSpendUnknown},
		{"instance type", nil, CreateRequest{Account: "a", Service: "ec2", InstanceType: "p4d.24xlarge", Tags: tagged(cfg)}, fakeSpend{}, CodeInstanceType},
		{"instance ok", nil, CreateRequest{Account: "a", Service: "ec2", InstanceType: "t3.micro", Tags: tagged(cfg)}, fakeSpend{}, ""},
		{"tags missing", nil, CreateRequest{Account: "a", Service: "ecr", Tags: map[string]string{"x": "y"}}, fakeSpend{}, CodeTagsMissing},
		{"no account", nil, CreateRequest{Service: "ecr", Tags: tagged(cfg)}, fakeSpend{}, CodeAccountMissing},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := baseCfg()
			if tc.mod != nil {
				tc.mod(&c)
			}
			d := New(func() Config { return c }, tc.spend).Check(context.Background(), tc.req)
			if tc.want == "" {
				if !d.Allowed {
					t.Fatalf("want allow, got %+v", d)
				}
				return
			}
			if d.Allowed || d.Code != tc.want || d.Message != refusals[tc.want] {
				t.Fatalf("want %s, got %+v", tc.want, d)
			}
		})
	}
}

func TestInjectTagsWinsAndExpires(t *testing.T) {
	cfg := baseCfg()
	got := InjectTags(cfg, map[string]string{"levelrail:owner": "forged", "name": "x"}, TagMeta{Owner: "real", Env: "e", Project: "p", ID: "i"})
	if got["levelrail:owner"] != "real" || got["levelrail:managed"] != "true" || got["name"] != "x" {
		t.Fatalf("got %v", got)
	}
	if got["levelrail:expires"] != "2026-11-22T12:00:00Z" {
		t.Fatalf("expires %q", got["levelrail:expires"])
	}
	cfg.DefaultTTL = 0
	if _, ok := InjectTags(cfg, nil, TagMeta{})["levelrail:expires"]; ok {
		t.Fatal("zero ttl must omit expires")
	}
}

func TestPlanExpiredDryRun(t *testing.T) {
	c, rec := awsxtest.New(t, func(awsxtest.Call, int) awsxtest.Reply {
		return awsxtest.Reply{Body: `{"ResourceTagMappingList":[
			{"ResourceARN":"arn:old","Tags":[{"Key":"levelrail:expires","Value":"2026-11-20T00:00:00Z"}]},
			{"ResourceARN":"arn:new","Tags":[{"Key":"levelrail:expires","Value":"2026-11-30T00:00:00Z"}]},
			{"ResourceARN":"arn:bad","Tags":[{"Key":"levelrail:expires","Value":"soon"}]}]}`}
	})
	plan, err := PlanExpired(context.Background(), c, baseCfg(), "us-east-1", 2)
	if err != nil || !plan.DryRun || len(plan.Items) != 1 || plan.Items[0].ARN != "arn:old" || len(plan.Unparsed) != 1 {
		t.Fatalf("got %v %+v", err, plan)
	}
	call := rec.Calls()[0]
	if call.Target != "ResourceGroupsTaggingAPI_20170126.GetResources" || !strings.Contains(call.Body, `"Key":"levelrail:managed"`) {
		t.Fatalf("call %+v", call)
	}
}

func TestFindOrphans(t *testing.T) {
	c, rec := awsxtest.New(t, func(call awsxtest.Call, _ int) awsxtest.Reply {
		switch {
		case strings.Contains(call.Body, "Action=DescribeVolumes") && strings.Contains(call.Body, "Value.1=available"):
			return awsxtest.Reply{Body: `<DescribeVolumesResponse><volumeSet><item><volumeId>vol-free</volumeId><size>8</size><volumeType>gp3</volumeType></item></volumeSet></DescribeVolumesResponse>`}
		case strings.Contains(call.Body, "Action=DescribeVolumes"):
			return awsxtest.Reply{Body: `<DescribeVolumesResponse><volumeSet><item><volumeId>vol-free</volumeId></item><item><volumeId>vol-live</volumeId></item></volumeSet></DescribeVolumesResponse>`}
		case strings.Contains(call.Body, "Action=DescribeAddresses"):
			return awsxtest.Reply{Body: `<DescribeAddressesResponse><addressesSet><item><publicIp>1.2.3.4</publicIp><allocationId>eipalloc-1</allocationId></item><item><publicIp>5.6.7.8</publicIp><allocationId>eipalloc-2</allocationId><associationId>eipassoc-1</associationId></item></addressesSet></DescribeAddressesResponse>`}
		}
		return awsxtest.Reply{Body: `<DescribeSnapshotsResponse><snapshotSet><item><snapshotId>snap-orphan</snapshotId><volumeId>vol-gone</volumeId></item><item><snapshotId>snap-ok</snapshotId><volumeId>vol-live</volumeId></item></snapshotSet></DescribeSnapshotsResponse>`}
	})
	rep, err := FindOrphans(context.Background(), c, "us-east-1", 1)
	if err != nil || !rep.DryRun || len(rep.Volumes) != 1 || rep.Volumes[0].ID != "vol-free" ||
		len(rep.Addresses) != 1 || rep.Addresses[0].ID != "eipalloc-1" || len(rep.Snapshots) != 1 || rep.Snapshots[0].ID != "snap-orphan" {
		t.Fatalf("got %v %+v", err, rep)
	}
	for _, call := range rec.Calls() {
		if strings.Contains(call.Body, "Delete") || strings.Contains(call.Body, "Release") {
			t.Fatalf("orphan finder must not mutate: %s", call.Body)
		}
	}
}

func TestSweepErrorClass(t *testing.T) {
	c, _ := awsxtest.New(t, func(awsxtest.Call, int) awsxtest.Reply {
		return awsxtest.Reply{Status: 403, Header: map[string]string{"X-Amzn-Errortype": "AccessDeniedException"}, Body: `{}`}
	})
	_, err := PlanExpired(context.Background(), c, baseCfg(), "us-east-1", 1)
	if awsx.ClassOf(err) != awsx.ClassAuth {
		t.Fatalf("got %v", err)
	}
}
