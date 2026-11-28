package cloudwatch

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/awsx/awsxtest"
)

var t0 = time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)

func TestGetMetricDataParsesAndBatches(t *testing.T) {
	c, rec := awsxtest.New(t, func(_ awsxtest.Call, n int) awsxtest.Reply {
		if n > 1 {
			return awsxtest.Reply{Body: `<R/>`}
		}
		return awsxtest.Reply{Body: `<GetMetricDataResponse><GetMetricDataResult><MetricDataResults><member><Id>m0</Id><Label>CPU</Label><StatusCode>Complete</StatusCode>` +
			`<Timestamps><member>2026-11-21T11:10:00Z</member><member>2026-11-21T11:05:00Z</member></Timestamps><Values><member>2.5</member><member>1.5</member></Values></member></MetricDataResults></GetMetricDataResult></GetMetricDataResponse>`}
	})
	qs := make([]Query, 501)
	for i := range qs {
		qs[i] = Query{ID: "m" + strconv.Itoa(i), Namespace: "AWS/EC2", Name: "CPUUtilization", Period: 300, Dimensions: []Dimension{{"InstanceId", "i-1"}}}
	}
	res, err := New(c).GetMetricData(context.Background(), "us-east-1", qs, t0.Add(-3*time.Hour), t0)
	if err != nil || len(res) != 501 || res[500].Points != nil {
		t.Fatalf("res=%d err=%v", len(res), err)
	}
	if len(res[0].Points) != 2 || res[0].Points[0].V != 1.5 || res[0].Status != "Complete" {
		t.Fatalf("points not sorted oldest first: %+v", res[0])
	}
	if len(rec.Calls()) != 2 {
		t.Fatalf("calls = %d, want 2 batches", len(rec.Calls()))
	}
	v, _ := url.ParseQuery(rec.Calls()[0].Body)
	if v.Get("Action") != "GetMetricData" || v.Get("MetricDataQueries.member.1.MetricStat.Metric.Dimensions.member.1.Name") != "InstanceId" ||
		v.Get("MetricDataQueries.member.1.MetricStat.Period") != "300" || v.Get("MetricDataQueries.member.1.MetricStat.Stat") != "Average" {
		t.Fatalf("form = %v", v)
	}
	if _, err := New(c).GetMetricData(context.Background(), "us-east-1", []Query{{ID: "x"}}, t0, t0); !errors.Is(err, ErrBadQuery) {
		t.Fatalf("bad query err = %v", err)
	}
}

func TestListMetrics(t *testing.T) {
	c, _ := awsxtest.New(t, func(awsxtest.Call, int) awsxtest.Reply {
		return awsxtest.Reply{Body: `<R><ListMetricsResult><Metrics><member><Namespace>AWS/EC2</Namespace><MetricName>CPUUtilization</MetricName><Dimensions><member><Name>InstanceId</Name><Value>i-1</Value></member></Dimensions></member></Metrics></ListMetricsResult></R>`}
	})
	got, err := New(c).ListMetrics(context.Background(), "us-east-1", "AWS/EC2", "", nil)
	if err != nil || len(got) != 1 || got[0].Dimensions[0].Value != "i-1" {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}

func TestCacheSingleFlightAndTTL(t *testing.T) {
	cache := NewCache(5 * time.Minute)
	clock := t0
	cache.now = func() time.Time { return clock }
	var calls int32
	gate := make(chan struct{})
	fn := func() (int, error) {
		atomic.AddInt32(&calls, 1)
		<-gate
		if calls < 0 {
			return 0, ErrBadQuery
		}
		return 7, nil
	}
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if v, _, err := Cached(cache, "k", fn); err != nil || v != 7 {
				t.Errorf("v=%d err=%v", v, err)
			}
		}()
	}
	for atomic.LoadInt32(&calls) == 0 {
		time.Sleep(time.Millisecond)
	}
	time.Sleep(20 * time.Millisecond)
	close(gate)
	wg.Wait()
	if calls != 1 {
		t.Fatalf("concurrent callers made %d provider calls", calls)
	}
	if _, hit, _ := Cached(cache, "k", fn); !hit || calls != 1 {
		t.Fatal("second read must come from the cache")
	}
	clock = clock.Add(6 * time.Minute)
	if _, hit, _ := Cached(cache, "k", fn); hit || calls != 2 {
		t.Fatalf("expired entry must refetch, hit=%v calls=%d", hit, calls)
	}
}

func TestLogsGroupsAndEventsPaginate(t *testing.T) {
	c, rec := awsxtest.New(t, func(call awsxtest.Call, n int) awsxtest.Reply {
		switch {
		case strings.HasSuffix(call.Target, "DescribeLogGroups"):
			return awsxtest.Reply{Body: `{"logGroups":[{"logGroupName":"/app","retentionInDays":14,"storedBytes":99}]}`}
		case n == 2:
			return awsxtest.Reply{Body: `{"events":[{"logStreamName":"s","timestamp":1790000000000,"message":"a"},{"logStreamName":"s","timestamp":1790000001000,"message":"b"}],"nextToken":"n1"}`}
		}
		return awsxtest.Reply{Body: `{"events":[{"logStreamName":"s","timestamp":1790000002000,"message":"c"}]}`}
	})
	cl := New(c)
	g, err := cl.DescribeLogGroups(context.Background(), "us-east-1", "/a", 10)
	if err != nil || len(g) != 1 || g[0].RetentionDays != 14 {
		t.Fatalf("groups=%+v err=%v", g, err)
	}
	ev, trunc, err := cl.FilterLogEvents(context.Background(), "us-east-1", FilterInput{Group: "/app", Start: t0.Add(-time.Hour), End: t0, Limit: 100})
	if err != nil || len(ev) != 3 || trunc || ev[2].Message != "c" {
		t.Fatalf("ev=%+v trunc=%v err=%v", ev, trunc, err)
	}
	if !strings.Contains(rec.Calls()[2].Body, `"nextToken":"n1"`) {
		t.Fatalf("second page must carry the token: %s", rec.Calls()[2].Body)
	}
	c2, _ := awsxtest.New(t, func(awsxtest.Call, int) awsxtest.Reply {
		return awsxtest.Reply{Body: `{"events":[{"logStreamName":"s","timestamp":1790000000000,"message":"a"},{"logStreamName":"s","timestamp":1790000001000,"message":"b"}],"nextToken":"more"}`}
	})
	ev, trunc, _ = New(c2).FilterLogEvents(context.Background(), "us-east-1", FilterInput{Group: "/app", Limit: 2})
	if len(ev) != 2 || !trunc {
		t.Fatalf("limit reached must report truncation, got %d %v", len(ev), trunc)
	}
}
