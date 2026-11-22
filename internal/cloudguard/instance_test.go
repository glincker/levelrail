package cloudguard

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/awsx/awsxtest"
)

const runOK = `<RunInstancesResponse><instancesSet><item><instanceId>i-0abc</instanceId></item></instancesSet></RunInstancesResponse>`

func runCase(t *testing.T, cfg Config, spend SpendSource, in InstanceInput) (string, []awsxtest.Call, error) {
	t.Helper()
	c, rec := awsxtest.New(t, func(awsxtest.Call, int) awsxtest.Reply { return awsxtest.Reply{Body: runOK} })
	id, err := RunInstance(context.Background(), c, New(func() Config { return cfg }, spend), in)
	return id, rec.Calls(), err
}

type fixedSpend float64

func (f fixedSpend) MonthToDateUSD(context.Context, string) (float64, error) { return float64(f), nil }

func baseIn() InstanceInput {
	return InstanceInput{Account: "acct", Region: "us-east-1", AMI: "ami-1", Type: "t3.micro", Name: "web",
		EstimatedMonthlyUSD: 8, Meta: TagMeta{Owner: "alice", Env: "dev", Project: "p", ID: "i1"}}
}

func TestRunInstanceTagsAndAllows(t *testing.T) {
	cfg := baseCfg()
	id, calls, err := runCase(t, cfg, fixedSpend(10), baseIn())
	if err != nil || id != "i-0abc" {
		t.Fatalf("id=%q err=%v", id, err)
	}
	if len(calls) != 1 {
		t.Fatalf("calls = %d", len(calls))
	}
	q, _ := url.ParseQuery(calls[0].Body)
	if q.Get("Action") != "RunInstances" || q.Get("InstanceType") != "t3.micro" {
		t.Fatalf("body = %s", calls[0].Body)
	}
	got := map[string]string{}
	for i := 1; i < 20; i++ {
		k := q.Get("TagSpecification.1.Tag." + string(rune('0'+i)) + ".Key")
		if k == "" {
			break
		}
		got[k] = q.Get("TagSpecification.1.Tag." + string(rune('0'+i)) + ".Value")
	}
	for _, s := range []string{"managed", "owner", "env", "expires"} {
		if got[TagKey(cfg, s)] == "" {
			t.Errorf("tag %s missing in %v", s, got)
		}
	}
	if got[TagKey(cfg, "owner")] != "alice" || got["Name"] != "web" || q.Get("TagSpecification.1.ResourceType") != "instance" {
		t.Fatalf("tags = %v", got)
	}
}

func TestRunInstanceRefusals(t *testing.T) {
	cases := []struct {
		name  string
		edit  func(*Config, *InstanceInput)
		spend float64
		want  string
	}{
		{"kill switch", func(c *Config, _ *InstanceInput) { c.KillSwitch = true }, 0, refusals[CodeKillSwitch]},
		{"type not allowed", func(_ *Config, i *InstanceInput) { i.Type = "p4d.24xlarge" }, 0, refusals[CodeInstanceType]},
		{"monthly cap", func(_ *Config, _ *InstanceInput) {}, 95, refusals[CodeMonthlyCap]},
		{"per create max", func(_ *Config, i *InstanceInput) { i.EstimatedMonthlyUSD = 50 }, 0, refusals[CodePerCreateMax]},
		{"service off", func(c *Config, _ *InstanceInput) { c.ServicesEnabled = map[string]bool{} }, 0, refusals[CodeServiceDisabled]},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, in := baseCfg(), baseIn()
			tc.edit(&cfg, &in)
			_, calls, err := runCase(t, cfg, fixedSpend(tc.spend), in)
			if !errors.Is(err, ErrRefused) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v", err)
			}
			if len(calls) != 0 {
				t.Fatalf("a refused create reached AWS: %d calls", len(calls))
			}
		})
	}
}

func TestRunInstanceFailsClosedWithoutSpend(t *testing.T) {
	_, calls, err := runCase(t, baseCfg(), nil, baseIn())
	if !errors.Is(err, ErrRefused) || len(calls) != 0 {
		t.Fatalf("err=%v calls=%d", err, len(calls))
	}
}
