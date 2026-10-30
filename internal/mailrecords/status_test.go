package mailrecords

import (
	"context"
	"slices"
	"testing"

	"github.com/GLINCKER/levelrail/internal/dnsstatus"
)

func TestPlanStatusAndHashExcludeLiveRecords(t *testing.T) {
	spec := Spec{Domain: "example.com", IPv4: "203.0.113.7"}
	p := &memProvider{}
	before, err := Plan(context.Background(), p, zone, spec, PlanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if before.ProviderSource != dnsstatus.SourceProvider || before.Counts.Waiting != before.Counts.Total || len(before.ToAdd) != before.Counts.Total {
		t.Fatalf("empty zone: counts %+v to_add %v", before.Counts, before.ToAdd)
	}
	desired, _, _ := Build(spec)
	var spf Record
	for _, d := range desired {
		if d.Kind == KindSPF {
			spf = d
		}
	}
	p.sets = append(p.sets, spf.Set)
	after, err := Plan(context.Background(), p, zone, spec, PlanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if after.Hash == before.Hash {
		t.Fatal("the hash must change when a record flips to live")
	}
	if slices.Contains(after.ToAdd, "spf-txt") || after.Counts.Live != 1 {
		t.Fatalf("a live record must leave to_add: %v %+v", after.ToAdd, after.Counts)
	}
	again, _ := Plan(context.Background(), p, zone, spec, PlanOptions{})
	if again.Hash != after.Hash {
		t.Fatal("the hash must be stable for the same zone")
	}
}

func TestPlanStatusDifferentIsNotAdded(t *testing.T) {
	res, err := Plan(context.Background(), foreignZone(), zone, Spec{Domain: "example.com", IPv4: "203.0.113.7"}, PlanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var mx dnsstatus.Record
	for _, r := range res.Records {
		if r.ID == "mx-mx" {
			mx = r
		}
	}
	if mx.Status != dnsstatus.Different || len(mx.Found) != 1 || mx.Found[0].Value != "1 aspmx.l.google.com" {
		t.Fatalf("foreign MX = %+v", mx)
	}
	if slices.Contains(res.ToAdd, "mx-mx") {
		t.Fatalf("a different record must not be added without replace_existing: %v", res.ToAdd)
	}
}
