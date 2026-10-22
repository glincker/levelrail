package dnszones

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// ProviderGoogleCloud names the Cloud DNS provider.
const ProviderGoogleCloud = "googleclouddns"

// googleCloudDNSBase and googleCloudDNSScope are Cloud DNS v1's endpoint and OAuth scope.
const (
	googleCloudDNSBase  = "https://dns.googleapis.com/dns/v1"
	googleCloudDNSScope = "https://www.googleapis.com/auth/ndev.clouddns.readwrite"
)

// GoogleCloudError is a non-2xx answer from Cloud DNS.
type GoogleCloudError struct {
	Status  int
	Message string
}

func (e *GoogleCloudError) Error() string {
	return fmt.Sprintf("googleclouddns: http %d: %s", e.Status, e.Message)
}

// GoogleCloud implements Provider over the Cloud DNS REST API. It supports
// simple routing only: no proxying, health checks or apex CNAME.
type GoogleCloud struct {
	BaseURL   string
	ProjectID string
	HTTP      *http.Client
}

type gcpKeyFile struct {
	ProjectID string `json:"project_id"`
}

// NewGoogleCloud builds a client from a service account JSON key, the same
// credential Compute Engine nodes use. The project comes from the key.
func NewGoogleCloud(serviceAccountJSON string) (*GoogleCloud, error) {
	var k gcpKeyFile
	if err := json.Unmarshal([]byte(serviceAccountJSON), &k); err != nil {
		return nil, fmt.Errorf("googleclouddns: credential must be a service account JSON key: %w", err)
	}
	if k.ProjectID == "" {
		return nil, errors.New("googleclouddns: credential is missing project_id")
	}
	cfg, err := google.JWTConfigFromJSON([]byte(serviceAccountJSON), googleCloudDNSScope)
	if err != nil {
		return nil, fmt.Errorf("googleclouddns: credential: %w", err)
	}
	return &GoogleCloud{
		BaseURL:   googleCloudDNSBase,
		ProjectID: k.ProjectID,
		HTTP:      oauth2.NewClient(context.Background(), cfg.TokenSource(context.Background())),
	}, nil
}

// Name implements Provider.
func (g *GoogleCloud) Name() string { return ProviderGoogleCloud }

// Capabilities implements Provider.
func (g *GoogleCloud) Capabilities() Capabilities { return Capabilities{} }

func (g *GoogleCloud) client() *http.Client {
	if g.HTTP != nil {
		return g.HTTP
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func (g *GoogleCloud) call(ctx context.Context, method, path string, body, out any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("googleclouddns: encode body: %w", err)
		}
		rdr = bytes.NewReader(b)
	}
	base := strings.TrimRight(g.BaseURL, "/")
	req, err := http.NewRequestWithContext(ctx, method, base+"/projects/"+url.PathEscape(g.ProjectID)+path, rdr)
	if err != nil {
		return fmt.Errorf("googleclouddns: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := g.client().Do(req)
	if err != nil {
		return fmt.Errorf("googleclouddns: %s %s: %w", method, strings.SplitN(path, "?", 2)[0], err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return fmt.Errorf("googleclouddns: read response: %w", err)
	}
	if resp.StatusCode >= 300 {
		var env struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(data, &env)
		msg := env.Error.Message
		if msg == "" {
			msg = http.StatusText(resp.StatusCode)
		}
		return &GoogleCloudError{Status: resp.StatusCode, Message: msg}
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("googleclouddns: decode response: %w", err)
		}
	}
	return nil
}

type gcdZone struct {
	Name        string   `json:"name"`
	DNSName     string   `json:"dnsName"`
	Description string   `json:"description,omitempty"`
	Visibility  string   `json:"visibility,omitempty"`
	NameServers []string `json:"nameServers,omitempty"`
	CreatedAt   string   `json:"creationTime,omitempty"`
}

func (z gcdZone) toZone() Zone {
	return Zone{
		ID:          z.Name,
		Name:        NormalizeDomain(z.DNSName),
		Provider:    ProviderGoogleCloud,
		Status:      "active",
		NameServers: z.NameServers,
		Private:     z.Visibility == "private",
		ModifiedAt:  z.CreatedAt,
	}
}

// ListZones implements Provider.
func (g *GoogleCloud) ListZones(ctx context.Context) ([]Zone, error) {
	var out []Zone
	token := ""
	for {
		var page struct {
			ManagedZones  []gcdZone `json:"managedZones"`
			NextPageToken string    `json:"nextPageToken"`
		}
		path := "/managedZones"
		if token != "" {
			path += "?pageToken=" + url.QueryEscape(token)
		}
		if err := g.call(ctx, http.MethodGet, path, nil, &page); err != nil {
			return nil, fmt.Errorf("list zones: %w", err)
		}
		for _, z := range page.ManagedZones {
			out = append(out, z.toZone())
		}
		if page.NextPageToken == "" {
			return out, nil
		}
		token = page.NextPageToken
	}
}

// GetZone implements Provider.
func (g *GoogleCloud) GetZone(ctx context.Context, id string) (Zone, error) {
	var z gcdZone
	if err := g.call(ctx, http.MethodGet, "/managedZones/"+url.PathEscape(id), nil, &z); err != nil {
		var ge *GoogleCloudError
		if errors.As(err, &ge) && ge.Status == http.StatusNotFound {
			return Zone{}, ErrZoneNotFound
		}
		return Zone{}, fmt.Errorf("get zone: %w", err)
	}
	return z.toZone(), nil
}

// CreateZone implements Provider. The managed zone id is the domain with dots turned into dashes.
func (g *GoogleCloud) CreateZone(ctx context.Context, name string) (Zone, error) {
	domain := NormalizeDomain(name)
	body := gcdZone{
		Name:        "zone-" + strings.ReplaceAll(domain, ".", "-"),
		DNSName:     domain + ".",
		Description: "managed zone for " + domain,
	}
	var z gcdZone
	if err := g.call(ctx, http.MethodPost, "/managedZones", body, &z); err != nil {
		return Zone{}, fmt.Errorf("create zone: %w", err)
	}
	return z.toZone(), nil
}

// DeleteZone implements Provider.
func (g *GoogleCloud) DeleteZone(ctx context.Context, id string) error {
	if err := g.call(ctx, http.MethodDelete, "/managedZones/"+url.PathEscape(id), nil, nil); err != nil {
		return fmt.Errorf("delete zone: %w", err)
	}
	return nil
}

type gcdRRSet struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	TTL     int      `json:"ttl"`
	RRDatas []string `json:"rrdatas"`
}

func fromGCD(s gcdRRSet, zone string) RecordSet {
	rs := RecordSet{Name: RelativeName(s.Name, zone), Type: s.Type, TTL: s.TTL, Routing: RoutingSimple}
	for _, v := range s.RRDatas {
		if s.Type == "TXT" {
			v = unquoteTXT(v)
		}
		rs.Values = append(rs.Values, v)
	}
	return rs
}

func toGCD(rs RecordSet, zone string) gcdRRSet {
	out := gcdRRSet{Name: FQDN(rs.Name, zone) + ".", Type: rs.Type, TTL: rs.TTL}
	for _, v := range rs.Values {
		if rs.Type == "TXT" {
			v = quoteTXT(v)
		}
		out.RRDatas = append(out.RRDatas, v)
	}
	return out
}

// ListRecordSets implements Provider.
func (g *GoogleCloud) ListRecordSets(ctx context.Context, zone Zone) ([]RecordSet, error) {
	var out []RecordSet
	token := ""
	for {
		var page struct {
			RRSets        []gcdRRSet `json:"rrsets"`
			NextPageToken string     `json:"nextPageToken"`
		}
		path := "/managedZones/" + url.PathEscape(zone.ID) + "/rrsets"
		if token != "" {
			path += "?pageToken=" + url.QueryEscape(token)
		}
		if err := g.call(ctx, http.MethodGet, path, nil, &page); err != nil {
			return nil, fmt.Errorf("list record sets: %w", err)
		}
		for _, s := range page.RRSets {
			out = append(out, fromGCD(s, zone.Name))
		}
		if page.NextPageToken == "" {
			return out, nil
		}
		token = page.NextPageToken
	}
}

func (g *GoogleCloud) existing(ctx context.Context, zone Zone, name, typ string) (*gcdRRSet, error) {
	q := url.Values{"name": {FQDN(name, zone.Name) + "."}, "type": {typ}}
	var page struct {
		RRSets []gcdRRSet `json:"rrsets"`
	}
	path := "/managedZones/" + url.PathEscape(zone.ID) + "/rrsets?" + q.Encode()
	if err := g.call(ctx, http.MethodGet, path, nil, &page); err != nil {
		return nil, fmt.Errorf("look up record set: %w", err)
	}
	if len(page.RRSets) == 0 {
		return nil, nil
	}
	return &page.RRSets[0], nil
}

func (g *GoogleCloud) change(ctx context.Context, zone Zone, add, del *gcdRRSet) error {
	body := map[string][]gcdRRSet{}
	if add != nil {
		body["additions"] = []gcdRRSet{*add}
	}
	if del != nil {
		body["deletions"] = []gcdRRSet{*del}
	}
	return g.call(ctx, http.MethodPost, "/managedZones/"+url.PathEscape(zone.ID)+"/changes", body, nil)
}

// UpsertRecordSet implements Provider by deleting any current set and adding the new one in one change.
func (g *GoogleCloud) UpsertRecordSet(ctx context.Context, zone Zone, rs RecordSet) error {
	cur, err := g.existing(ctx, zone, rs.Name, rs.Type)
	if err != nil {
		return err
	}
	add := toGCD(rs, zone.Name)
	if err := g.change(ctx, zone, &add, cur); err != nil {
		return fmt.Errorf("upsert %s %s: %w", rs.Name, rs.Type, err)
	}
	return nil
}

// DeleteRecordSet implements Provider.
func (g *GoogleCloud) DeleteRecordSet(ctx context.Context, zone Zone, k Key) error {
	cur, err := g.existing(ctx, zone, k.Name, k.Type)
	if err != nil {
		return err
	}
	if cur == nil {
		return nil
	}
	if err := g.change(ctx, zone, nil, cur); err != nil {
		return fmt.Errorf("delete %s %s: %w", k.Name, k.Type, err)
	}
	return nil
}
