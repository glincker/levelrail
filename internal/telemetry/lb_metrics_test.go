package telemetry

import (
	"context"
	"testing"
	"time"
)

func TestRecordLoadBalancer(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	at := time.Now().UTC().Truncate(time.Second)

	if err := db.RecordLoadBalancer(ctx, nil, at); err != nil {
		t.Fatalf("empty record error = %v", err)
	}
	if err := db.RecordLoadBalancer(ctx, []LoadBalancerSample{{Service: "web", Total: 3, Healthy: 2, ActiveRequests: 9}}, at); err != nil {
		t.Fatal(err)
	}
	for metric, want := range map[string]float64{MetricLBUpstreamsTotal: 3, MetricLBUpstreamsHealthy: 2, MetricLBActiveRequests: 9} {
		got, err := db.Query(ctx, "service:web", metric, at.Add(-time.Minute), at.Add(time.Minute))
		if err != nil || len(got) != 1 || got[0].Value != want {
			t.Errorf("%s = %v err %v, want one sample of %v", metric, got, err, want)
		}
	}
}
