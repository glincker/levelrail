package mailrecords

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"net"
	"slices"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/dnszones"
)

func lines(recs []Record) []string {
	var out []string
	for _, r := range recs {
		for _, v := range r.Set.Values {
			out = append(out, r.Set.Name+" "+r.Set.Type+" "+v)
		}
	}
	return out
}

func TestBuildServerModeGolden(t *testing.T) {
	recs, notes, err := Build(Spec{
		Domain: "example.com", IPv4: "203.0.113.7", IPv6: "2001:db8::7",
		Autoconfig: true, TLSRPT: true, MTASTSMode: "testing", MTASTSMaxAge: 86400, DKIMSelector: "s1",
	})
	if err != nil {
		t.Fatal(err)
	}
	got := lines(recs)
	want := []string{
		"mail A 203.0.113.7",
		"mail AAAA 2001:db8::7",
		"@ MX 10 mail.example.com",
		"@ TXT v=spf1 mx ip4:203.0.113.7 ip6:2001:db8::7 ~all",
		"_dmarc TXT v=DMARC1; p=none; rua=mailto:dmarc@example.com",
		"_mta-sts TXT v=STSv1; id=",
		"_smtp._tls TXT v=TLSRPTv1; rua=mailto:tlsrpt@example.com",
		"_imaps._tcp SRV 0 1 993 mail.example.com",
		"_submission._tcp SRV 0 1 587 mail.example.com",
		"autoconfig CNAME mail.example.com",
		"autodiscover CNAME mail.example.com",
	}
	for _, w := range want {
		if !slices.ContainsFunc(got, func(g string) bool { return strings.HasPrefix(g, w) }) {
			t.Errorf("missing %q in\n%s", w, strings.Join(got, "\n"))
		}
	}
	codes := map[string]bool{}
	for _, n := range notes {
		codes[n.Code] = true
	}
	for _, c := range []string{"dkim_pending", "mta_sts_policy"} {
		if !codes[c] {
			t.Errorf("missing note %s", c)
		}
	}
	for _, g := range got {
		if strings.Contains(g, "+all") {
			t.Fatal("never +all")
		}
	}
}

func TestBuildExternalPreset(t *testing.T) {
	recs, _, err := Build(Spec{Domain: "example.com", Preset: "fastmail"})
	if err != nil {
		t.Fatal(err)
	}
	got := lines(recs)
	for _, w := range []string{
		"@ MX 10 in1-smtp.messagingengine.com",
		"@ MX 20 in2-smtp.messagingengine.com",
		"@ TXT v=spf1 include:spf.messagingengine.com ~all",
		"fm1._domainkey CNAME fm1.example.com.dkim.fmhosted.com",
	} {
		if !slices.Contains(got, w) {
			t.Errorf("missing %q in\n%s", w, strings.Join(got, "\n"))
		}
	}
	if _, _, err := Build(Spec{Domain: "example.com", Preset: "nope"}); err == nil {
		t.Fatal("unknown preset must fail")
	}
}

func TestBuildRejectsBadNames(t *testing.T) {
	for _, s := range []Spec{
		{Domain: "example.com."}, {Domain: "example"}, {Domain: "example.com", MailHost: "mail.example.com."},
		{Domain: "example.com", MailHost: "mail"}, {Domain: "example.com", IPv4: "10.0.0.1"},
		{Domain: "example.com", SPFMode: SPFInclude}, {Domain: "example.com", DMARCPolicy: "weird"},
	} {
		if _, _, err := Build(s); err == nil {
			t.Errorf("expected error for %+v", s)
		}
	}
}

func TestBuildTXTChunkingNote(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, _ := x509.MarshalPKIXPublicKey(&key.PublicKey)
	recs, notes, err := Build(Spec{Domain: "example.com", DKIMSelector: "s1", DKIMPublicKey: base64.StdEncoding.EncodeToString(der)})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(recs, func(r Record) bool { return r.Kind == KindDKIM && len(r.Set.Values[0]) > 255 }) {
		t.Fatal("expected a long DKIM value")
	}
	if !slices.ContainsFunc(notes, func(n Note) bool { return n.Code == "txt_chunked" }) {
		t.Fatal("expected a chunking note")
	}
}

type memProvider struct {
	sets []dnszones.RecordSet
	ops  []string
}

func (m *memProvider) Name() string { return "mem" }
func (m *memProvider) Capabilities() dnszones.Capabilities {
	return dnszones.Capabilities{ApexCNAME: true}
}
func (m *memProvider) ListZones(context.Context) ([]dnszones.Zone, error) { return nil, nil }
func (m *memProvider) GetZone(context.Context, string) (dnszones.Zone, error) {
	return dnszones.Zone{}, nil
}
func (m *memProvider) CreateZone(context.Context, string) (dnszones.Zone, error) {
	return dnszones.Zone{}, nil
}
func (m *memProvider) DeleteZone(context.Context, string) error { return nil }
func (m *memProvider) ListRecordSets(context.Context, dnszones.Zone) ([]dnszones.RecordSet, error) {
	return slices.Clone(m.sets), nil
}
func (m *memProvider) UpsertRecordSet(_ context.Context, _ dnszones.Zone, rs dnszones.RecordSet) error {
	m.ops = append(m.ops, "upsert "+rs.Name+" "+rs.Type)
	m.sets = slices.DeleteFunc(m.sets, func(e dnszones.RecordSet) bool { return e.Key() == rs.Key() })
	m.sets = append(m.sets, rs)
	return nil
}
func (m *memProvider) DeleteRecordSet(_ context.Context, _ dnszones.Zone, k dnszones.Key) error {
	m.ops = append(m.ops, "delete "+k.Name+" "+k.Type)
	return nil
}

var zone = dnszones.Zone{ID: "z1", Name: "example.com"}

func foreignZone() *memProvider {
	return &memProvider{sets: []dnszones.RecordSet{
		{Name: "@", Type: "MX", TTL: 300, Values: []string{"1 aspmx.l.google.com"}},
		{Name: "@", Type: "TXT", TTL: 300, Values: []string{"v=spf1 include:_spf.google.com ~all", "google-site-verification=abc"}},
	}}
}

func TestPlanAddOnlyRefusesForeignMX(t *testing.T) {
	p := foreignZone()
	res, err := Plan(context.Background(), p, zone, Spec{Domain: "example.com", IPv4: "203.0.113.7"}, PlanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Blocked || !strings.Contains(strings.Join(res.Refusals, "\n"), "MX records pointing elsewhere") {
		t.Fatalf("expected MX refusal, got %+v", res.Refusals)
	}
	if _, _, err := Apply(context.Background(), p, zone, Spec{Domain: "example.com", IPv4: "203.0.113.7"}, PlanOptions{}, res.Hash); !errors.Is(err, ErrBlocked) {
		t.Fatalf("apply must refuse, got %v", err)
	}
	if len(p.ops) != 0 {
		t.Fatalf("nothing may be written: %v", p.ops)
	}
}

func TestPlanReplaceListsExactTypes(t *testing.T) {
	p := foreignZone()
	spec := Spec{Domain: "example.com", IPv4: "203.0.113.7"}
	res, err := Plan(context.Background(), p, zone, spec, PlanOptions{ReplaceExisting: []string{"mx", "SPF"}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Blocked {
		t.Fatalf("blocked: %v", res.Refusals)
	}
	var replaced []string
	for _, c := range res.Changes {
		if c.Action == ActionReplace {
			replaced = append(replaced, c.Kind)
		}
	}
	slices.Sort(replaced)
	if !slices.Equal(replaced, []string{"MX", "SPF"}) || !slices.Equal(res.Replacing, []string{"MX", "SPF"}) {
		t.Fatalf("replaced %v replacing %v", replaced, res.Replacing)
	}
	n, _, err := Apply(context.Background(), p, zone, spec, PlanOptions{ReplaceExisting: []string{"MX", "SPF"}}, res.Hash)
	if err != nil || n == 0 {
		t.Fatalf("apply n=%d err=%v", n, err)
	}
	for _, s := range p.sets {
		if s.Name == "@" && s.Type == "TXT" && !slices.Contains(s.Values, "google-site-verification=abc") {
			t.Fatal("unrelated apex TXT must survive an SPF replace")
		}
	}
}

func TestApplyRejectsStalePlanHash(t *testing.T) {
	p := &memProvider{}
	spec := Spec{Domain: "example.com", IPv4: "203.0.113.7"}
	res, err := Plan(context.Background(), p, zone, spec, PlanOptions{})
	if err != nil || res.Blocked {
		t.Fatalf("plan: %v %v", err, res.Refusals)
	}
	p.sets = append(p.sets, dnszones.RecordSet{Name: "@", Type: "MX", TTL: 300, Values: []string{"5 other.example.net"}})
	if _, _, err := Apply(context.Background(), p, zone, spec, PlanOptions{}, res.Hash); !errors.Is(err, ErrStalePlan) {
		t.Fatalf("want stale, got %v", err)
	}
	if len(p.ops) != 0 {
		t.Fatal("stale apply wrote records")
	}
}

type fakeResolver struct {
	txt   map[string][]string
	cname map[string]string
	mx    []*net.MX
	host  map[string][]string
	ptr   map[string][]string
}

func (f fakeResolver) LookupTXT(_ context.Context, n string) ([]string, error) { return f.txt[n], nil }
func (f fakeResolver) LookupCNAME(_ context.Context, n string) (string, error) {
	if c, ok := f.cname[n]; ok {
		return c, nil
	}
	return n + ".", nil
}
func (f fakeResolver) LookupMX(context.Context, string) ([]*net.MX, error) { return f.mx, nil }
func (f fakeResolver) LookupHost(_ context.Context, h string) ([]string, error) {
	return f.host[h], nil
}
func (f fakeResolver) LookupAddr(_ context.Context, a string) ([]string, error) { return f.ptr[a], nil }
func (f fakeResolver) LookupSRV(context.Context, string, string, string) (string, []*net.SRV, error) {
	return "", nil, errors.New("none")
}

func TestCheckWithFakeResolver(t *testing.T) {
	r := fakeResolver{
		txt: map[string][]string{
			"example.com":               {"v=spf1 mx ~all"},
			"_dmarc.example.com":        {"v=DMARC1; p=quarantine"},
			"s1._domainkey.example.com": {"v=DKIM1; k=rsa; p=AAAA"},
		},
		mx:   []*net.MX{{Host: "mail.example.com.", Pref: 10}},
		host: map[string][]string{"mail.example.com": {"203.0.113.7"}},
		ptr:  map[string][]string{"203.0.113.7": {"other.example.net."}},
	}
	rep, err := Check(context.Background(), r, "example.com", Expect{MailHost: "mail.example.com", DKIMSelector: "s1"})
	if err != nil {
		t.Fatal(err)
	}
	st := map[string]string{}
	for _, it := range rep.Items {
		st[it.ID] = it.Status
	}
	want := map[string]string{
		"mx": StatusPass, "spf": StatusPass, "dmarc": StatusPass, "dkim": StatusPass,
		"mta_sts": StatusWarn, "tls_rpt": StatusWarn, "autoconfig": StatusWarn,
		"fcrdns": StatusFail, "port25": StatusNotTested,
	}
	for k, v := range want {
		if st[k] != v {
			t.Errorf("%s: got %q want %q", k, st[k], v)
		}
	}
}

func TestCheckDKIMProxiedFails(t *testing.T) {
	r := fakeResolver{host: map[string][]string{"s1._domainkey.example.com": {"104.21.5.9"}}}
	it := checkDKIM(context.Background(), r, "example.com", "s1")
	if it.Status != StatusFail || !strings.Contains(it.Message, "Cloudflare") {
		t.Fatalf("got %+v", it)
	}
}

func TestCheckSPFTooManyLookupsFails(t *testing.T) {
	r := fakeResolver{txt: map[string][]string{"example.com": {"v=spf1 a mx a mx a mx a mx a mx a ~all"}}}
	rep, err := Check(context.Background(), r, "example.com", Expect{})
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range rep.Items {
		if it.ID == "spf" && it.Status != StatusFail {
			t.Fatalf("spf = %+v", it)
		}
	}
}
