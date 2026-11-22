package cloudguard

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/awsx/awsxtest"
)

func metricResult(id string, vals ...string) string {
	ts, vs := "", ""
	for i, v := range vals {
		ts += "<member>2026-11-" + string(rune('1'+i%9)) + "T00:00:00Z</member>"
		vs += "<member>" + v + "</member>"
	}
	return `<member><Id>` + id + `</Id><StatusCode>Complete</StatusCode><Timestamps>` + ts + `</Timestamps><Values>` + vs + `</Values></member>`
}

func TestFindIdleTable(t *testing.T) {
	c, rec := awsxtest.New(t, func(call awsxtest.Call, _ int) awsxtest.Reply {
		f, _ := url.ParseQuery(call.Body)
		switch f.Get("Action") {
		case "DescribeInstances":
			return awsxtest.Reply{Body: `<R><reservationSet><item><instancesSet><item><instanceId>i-idle</instanceId><instanceType>t3.micro</instanceType></item></instancesSet></item>` +
				`<item><instancesSet><item><instanceId>i-busy</instanceId><instanceType>t3.micro</instanceType></item><item><instanceId>i-new</instanceId><instanceType>t3.micro</instanceType></item></instancesSet></item></reservationSet></R>`}
		case "DescribeVolumes":
			return awsxtest.Reply{Body: `<R><volumeSet><item><volumeId>vol-free</volumeId><size>100</size><volumeType>gp3</volumeType></item></volumeSet></R>`}
		case "DescribeAddresses":
			return awsxtest.Reply{Body: `<R><addressesSet><item><publicIp>1.2.3.4</publicIp><allocationId>eipalloc-1</allocationId></item><item><publicIp>5.6.7.8</publicIp><allocationId>eipalloc-2</allocationId><associationId>eipassoc-1</associationId></item></addressesSet></R>`}
		case "DescribeLoadBalancers":
			return awsxtest.Reply{Body: `<R><DescribeLoadBalancersResult><LoadBalancers>` +
				`<member><LoadBalancerArn>arn:aws:elasticloadbalancing:us-east-1:1:loadbalancer/app/quiet/abc</LoadBalancerArn><LoadBalancerName>quiet</LoadBalancerName><Type>application</Type><CreatedTime>2026-01-01T00:00:00Z</CreatedTime></member>` +
				`<member><LoadBalancerArn>arn:aws:elasticloadbalancing:us-east-1:1:loadbalancer/app/busy/def</LoadBalancerArn><LoadBalancerName>busy</LoadBalancerName><Type>application</Type><CreatedTime>2026-01-01T00:00:00Z</CreatedTime></member>` +
				`<member><LoadBalancerArn>arn:aws:elasticloadbalancing:us-east-1:1:loadbalancer/net/nlb/ghi</LoadBalancerArn><LoadBalancerName>nlb</LoadBalancerName><Type>network</Type><CreatedTime>2026-01-01T00:00:00Z</CreatedTime></member>` +
				`<member><LoadBalancerArn>arn:aws:elasticloadbalancing:us-east-1:1:loadbalancer/app/fresh/jkl</LoadBalancerArn><LoadBalancerName>fresh</LoadBalancerName><Type>application</Type><CreatedTime>2026-11-20T00:00:00Z</CreatedTime></member>` +
				`</LoadBalancers></DescribeLoadBalancersResult></R>`}
		case "GetMetricData":
			if f.Get("MetricDataQueries.member.1.MetricStat.Metric.Namespace") == "AWS/EC2" {
				return awsxtest.Reply{Body: `<R><GetMetricDataResult><MetricDataResults>` + metricResult("i0", "1", "2", "1", "1", "2", "1", "1") +
					metricResult("i1", "1", "60", "1", "1", "2", "1", "1") + metricResult("i2", "1", "1") + `</MetricDataResults></GetMetricDataResult></R>`}
			}
			return awsxtest.Reply{Body: `<R><GetMetricDataResult><MetricDataResults>` + metricResult("l0") + metricResult("l1", "10", "20") + `</MetricDataResults></GetMetricDataResult></R>`}
		}
		return awsxtest.Reply{Body: "<R/>"}
	})
	cfg := IdleConfig{CPUPercent: 5, Days: 7, EBSGBMonthUSD: 0.08, EIPMonthUSD: 3.65, ALBMonthUSD: 16, Now: func() time.Time { return now }}
	got, err := FindIdle(context.Background(), c, cfg, "us-east-1", 3)
	if err != nil {
		t.Fatal(err)
	}
	have := map[string]Suggestion{}
	for _, s := range got {
		have[s.Kind+"/"+s.ID] = s
	}
	want := []string{"ec2_low_cpu/i-idle", "ebs_unattached/vol-free", "eip_unassociated/eipalloc-1", "alb_no_requests/quiet"}
	for _, k := range want {
		if _, ok := have[k]; !ok {
			t.Errorf("missing suggestion %s in %v", k, have)
		}
	}
	for _, k := range []string{"ec2_low_cpu/i-busy", "ec2_low_cpu/i-new", "eip_unassociated/eipalloc-2", "alb_no_requests/busy", "alb_no_requests/nlb", "alb_no_requests/fresh"} {
		if _, ok := have[k]; ok {
			t.Errorf("unexpected suggestion %s", k)
		}
	}
	if v := have["ebs_unattached/vol-free"]; v.MonthlyUSD != 8 || v.Action != ActionSnapshotDelete {
		t.Errorf("volume suggestion = %+v", v)
	}
	if a := have["ec2_low_cpu/i-idle"].Action; a != ActionStop {
		t.Errorf("cpu action = %q", a)
	}
	for _, call := range rec.Calls() {
		f, _ := url.ParseQuery(call.Body)
		if a := f.Get("Action"); !strings.HasPrefix(a, "Describe") && a != "GetMetricData" {
			t.Fatalf("idle detection issued a non read action %s", a)
		}
	}
}
