package dnszones

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/route53/types"
)

type r53Fake struct {
	sets     []types.ResourceRecordSet
	changes  []types.Change
	hcs      []types.HealthCheck
	denyList bool
}

func (f *r53Fake) ListHostedZones(context.Context, *route53.ListHostedZonesInput, ...func(*route53.Options)) (*route53.ListHostedZonesOutput, error) {
	if f.denyList {
		return nil, errors.New("AccessDenied")
	}
	return &route53.ListHostedZonesOutput{HostedZones: []types.HostedZone{
		{Id: aws.String("/hostedzone/Z1"), Name: aws.String("example.com."), ResourceRecordSetCount: aws.Int64(4)},
		{Id: aws.String("/hostedzone/Z2"), Name: aws.String("internal.example."), Config: &types.HostedZoneConfig{PrivateZone: true}},
	}}, nil
}

func (f *r53Fake) GetHostedZone(_ context.Context, in *route53.GetHostedZoneInput, _ ...func(*route53.Options)) (*route53.GetHostedZoneOutput, error) {
	if aws.ToString(in.Id) != "Z1" {
		return nil, errors.New("NoSuchHostedZone")
	}
	return &route53.GetHostedZoneOutput{
		HostedZone:    &types.HostedZone{Id: aws.String("/hostedzone/Z1"), Name: aws.String("example.com.")},
		DelegationSet: &types.DelegationSet{NameServers: []string{"ns-1.awsdns-01.org", "ns-2.awsdns-02.com"}},
	}, nil
}

func (f *r53Fake) CreateHostedZone(_ context.Context, in *route53.CreateHostedZoneInput, _ ...func(*route53.Options)) (*route53.CreateHostedZoneOutput, error) {
	if in.CallerReference == nil {
		return nil, errors.New("CallerReference required")
	}
	return &route53.CreateHostedZoneOutput{
		HostedZone:    &types.HostedZone{Id: aws.String("/hostedzone/Z9"), Name: in.Name},
		DelegationSet: &types.DelegationSet{NameServers: []string{"ns-9.awsdns-09.net"}},
	}, nil
}

func (f *r53Fake) DeleteHostedZone(context.Context, *route53.DeleteHostedZoneInput, ...func(*route53.Options)) (*route53.DeleteHostedZoneOutput, error) {
	return &route53.DeleteHostedZoneOutput{}, nil
}

func (f *r53Fake) ListResourceRecordSets(context.Context, *route53.ListResourceRecordSetsInput, ...func(*route53.Options)) (*route53.ListResourceRecordSetsOutput, error) {
	return &route53.ListResourceRecordSetsOutput{ResourceRecordSets: slices.Clone(f.sets)}, nil
}

func (f *r53Fake) ChangeResourceRecordSets(_ context.Context, in *route53.ChangeResourceRecordSetsInput, _ ...func(*route53.Options)) (*route53.ChangeResourceRecordSetsOutput, error) {
	for _, c := range in.ChangeBatch.Changes {
		f.changes = append(f.changes, c)
		s := *c.ResourceRecordSet
		f.sets = slices.DeleteFunc(f.sets, func(e types.ResourceRecordSet) bool {
			return aws.ToString(e.Name) == aws.ToString(s.Name) && e.Type == s.Type && aws.ToString(e.SetIdentifier) == aws.ToString(s.SetIdentifier)
		})
		if c.Action == types.ChangeActionUpsert {
			f.sets = append(f.sets, s)
		}
	}
	return &route53.ChangeResourceRecordSetsOutput{}, nil
}

func (f *r53Fake) ListHealthChecks(context.Context, *route53.ListHealthChecksInput, ...func(*route53.Options)) (*route53.ListHealthChecksOutput, error) {
	return &route53.ListHealthChecksOutput{HealthChecks: f.hcs}, nil
}

func (f *r53Fake) CreateHealthCheck(_ context.Context, in *route53.CreateHealthCheckInput, _ ...func(*route53.Options)) (*route53.CreateHealthCheckOutput, error) {
	hc := types.HealthCheck{Id: aws.String("hc-1"), HealthCheckConfig: in.HealthCheckConfig}
	f.hcs = append(f.hcs, hc)
	return &route53.CreateHealthCheckOutput{HealthCheck: &hc}, nil
}

func (f *r53Fake) DeleteHealthCheck(context.Context, *route53.DeleteHealthCheckInput, ...func(*route53.Options)) (*route53.DeleteHealthCheckOutput, error) {
	f.hcs = nil
	return &route53.DeleteHealthCheckOutput{}, nil
}

func TestRoute53Zones(t *testing.T) {
	ctx := context.Background()
	fake := &r53Fake{}
	r := &Route53{API: fake}
	zones, err := r.ListZones(ctx)
	if err != nil || len(zones) != 2 {
		t.Fatalf("zones = %+v %v", zones, err)
	}
	if zones[0].ID != "Z1" || *zones[0].RecordCount != 4 || len(zones[0].NameServers) != 2 || !zones[1].Private {
		t.Errorf("zones = %+v", zones)
	}
	z, err := r.CreateZone(ctx, "new.example")
	if err != nil || z.ID != "Z9" || z.NameServers[0] != "ns-9.awsdns-09.net" {
		t.Errorf("create = %+v %v", z, err)
	}
	fake.denyList = true
	r.HostedZoneID = "/hostedzone/Z1"
	if zones, err := r.ListZones(ctx); err != nil || len(zones) != 1 {
		t.Errorf("fallback to configured zone failed: %v %v", zones, err)
	}
	r.HostedZoneID = ""
	if _, err := r.ListZones(ctx); err == nil {
		t.Error("denied list without a configured zone must fail")
	}
}

func TestRoute53RecordSets(t *testing.T) {
	ctx := context.Background()
	fake := &r53Fake{sets: []types.ResourceRecordSet{
		{Name: aws.String("example.com."), Type: types.RRTypeNs, TTL: aws.Int64(172800), ResourceRecords: []types.ResourceRecord{{Value: aws.String("ns-1.awsdns-01.org.")}}},
		{Name: aws.String(`\052.example.com.`), Type: types.RRTypeA, TTL: aws.Int64(60), ResourceRecords: []types.ResourceRecord{{Value: aws.String("1.2.3.4")}}},
		{Name: aws.String("example.com."), Type: types.RRTypeA, AliasTarget: &types.AliasTarget{DNSName: aws.String("lb.amazonaws.com."), HostedZoneId: aws.String("ZLB")}},
		{Name: aws.String("example.com."), Type: types.RRTypeTxt, TTL: aws.Int64(300), ResourceRecords: []types.ResourceRecord{{Value: aws.String(`"v=spf1 " "-all"`)}}},
	}}
	r := &Route53{API: fake}
	zone := Zone{ID: "Z1", Name: "example.com"}
	got, err := r.ListRecordSets(ctx, zone)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(got, func(s RecordSet) bool { return s.Name == "*" && s.Type == "A" }) {
		t.Errorf("\\052 must decode to *: %+v", got)
	}
	if !slices.ContainsFunc(got, func(s RecordSet) bool { return s.Alias != nil && s.Alias.DNSName == "lb.amazonaws.com" }) {
		t.Errorf("alias missing: %+v", got)
	}
	if !slices.ContainsFunc(got, func(s RecordSet) bool { return s.Type == "TXT" && s.Values[0] == "v=spf1 -all" }) {
		t.Errorf("txt not unquoted: %+v", got)
	}

	primary := RecordSet{Name: "app", Type: "A", TTL: 60, Values: []string{"1.1.1.1"}, Routing: RoutingFailover, SetIdentifier: "p", Failover: FailoverPrimary, HealthCheckID: "hc-1"}
	weighted := RecordSet{Name: "lb", Type: "A", TTL: 60, Values: []string{"2.2.2.2"}, Routing: RoutingWeighted, SetIdentifier: "w1", Weight: w(70)}
	multi := RecordSet{Name: "mv", Type: "A", TTL: 60, Values: []string{"3.3.3.3"}, Routing: RoutingMultivalue, SetIdentifier: "m1"}
	txt := RecordSet{Name: "t", Type: "TXT", TTL: 60, Routing: RoutingSimple, Values: []string{`say "hi"`}}
	for _, s := range []RecordSet{primary, weighted, multi, txt} {
		if err := r.UpsertRecordSet(ctx, zone, s); err != nil {
			t.Fatal(err)
		}
	}
	last := fake.changes[len(fake.changes)-1].ResourceRecordSet
	if aws.ToString(last.ResourceRecords[0].Value) != `"say \"hi\""` {
		t.Errorf("txt write = %s", aws.ToString(last.ResourceRecords[0].Value))
	}
	got, _ = r.ListRecordSets(ctx, zone)
	for _, want := range []RecordSet{primary, weighted, multi, txt} {
		i := slices.IndexFunc(got, func(g RecordSet) bool { return g.Key() == want.Key() })
		if i < 0 || !SameContent(got[i], want) || got[i].Routing != want.Routing {
			t.Errorf("%s: got %+v", want.Name, got)
		}
	}
	if err := r.DeleteRecordSet(ctx, zone, primary.Key()); err != nil {
		t.Fatal(err)
	}
	del := fake.changes[len(fake.changes)-1]
	if del.Action != types.ChangeActionDelete || aws.ToString(del.ResourceRecordSet.HealthCheckId) != "hc-1" {
		t.Errorf("delete must send the exact current set, got %+v", del)
	}
	if err := r.DeleteRecordSet(ctx, zone, Key{Name: "missing", Type: "A"}); err != nil {
		t.Errorf("deleting a missing set is not an error: %v", err)
	}
}

func TestRoute53HealthChecks(t *testing.T) {
	ctx := context.Background()
	r := &Route53{API: &r53Fake{}}
	hc, err := r.CreateHealthCheck(ctx, HealthCheck{Type: "https", FQDN: "app.example.com", Port: 443, ResourcePath: "/healthz"})
	if err != nil || hc.ID != "hc-1" {
		t.Fatalf("create = %+v %v", hc, err)
	}
	list, err := r.ListHealthChecks(ctx)
	if err != nil || len(list) != 1 || list[0].Type != "HTTPS" || list[0].ResourcePath != "/healthz" {
		t.Fatalf("list = %+v %v", list, err)
	}
	if err := r.DeleteHealthCheck(ctx, "hc-1"); err != nil {
		t.Fatal(err)
	}
}
