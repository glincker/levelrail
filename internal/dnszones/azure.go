package dnszones

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/oauth2/clientcredentials"
)

// ProviderAzure names the Azure DNS provider.
const ProviderAzure = "azure"

const (
	azureARMBase    = "https://management.azure.com"
	azureLoginBase  = "https://login.microsoftonline.com"
	azureAPIVersion = "2018-05-01"
)

// AzureCredential is the service principal and scope Azure DNS needs.
type AzureCredential struct {
	TenantID       string `json:"tenant_id"`
	ClientID       string `json:"client_id"`
	ClientSecret   string `json:"client_secret"`
	SubscriptionID string `json:"subscription_id"`
	ResourceGroup  string `json:"resource_group"`
}

// ParseAzureCredential decodes and validates the stored credential JSON without echoing it.
func ParseAzureCredential(raw string) (AzureCredential, error) {
	var c AzureCredential
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return c, errors.New("azure: credential must be JSON with tenant_id, client_id, client_secret, subscription_id and resource_group")
	}
	for name, v := range map[string]string{"tenant_id": c.TenantID, "client_id": c.ClientID, "client_secret": c.ClientSecret, "subscription_id": c.SubscriptionID, "resource_group": c.ResourceGroup} {
		if strings.TrimSpace(v) == "" {
			return c, fmt.Errorf("azure: credential is missing %s", name)
		}
	}
	return c, nil
}

// Azure implements Provider over the Azure DNS resource manager API.
type Azure struct {
	rc   restClient
	cred AzureCredential
}

// NewAzure builds a client from the credential JSON. Tokens come from the client credentials flow.
func NewAzure(rawCredential string) (*Azure, error) {
	c, err := ParseAzureCredential(rawCredential)
	if err != nil {
		return nil, err
	}
	cfg := clientcredentials.Config{
		ClientID: c.ClientID, ClientSecret: c.ClientSecret,
		TokenURL: azureLoginBase + "/" + url.PathEscape(c.TenantID) + "/oauth2/v2.0/token",
		Scopes:   []string{azureARMBase + "/.default"},
	}
	hc := cfg.Client(context.Background())
	hc.Timeout = 30 * time.Second
	return &Azure{cred: c, rc: restClient{provider: ProviderAzure, base: azureARMBase, http: hc}}, nil
}

// SetBaseURL points the client at another API root and HTTP client (tests).
func (a *Azure) SetBaseURL(base string, c *http.Client, cred AzureCredential) {
	a.rc.base, a.rc.http, a.cred = base, c, cred
}

// Name implements Provider.
func (a *Azure) Name() string { return ProviderAzure }

// Capabilities implements Provider. Azure DNS has no apex CNAME.
func (a *Azure) Capabilities() Capabilities { return Capabilities{} }

func (a *Azure) zonesPath() string {
	return "/subscriptions/" + url.PathEscape(a.cred.SubscriptionID) + "/resourceGroups/" + url.PathEscape(a.cred.ResourceGroup) + "/providers/Microsoft.Network/dnsZones"
}

type azZone struct {
	Name       string `json:"name"`
	Properties struct {
		NameServers []string `json:"nameServers"`
		NumberOf    int      `json:"numberOfRecordSets"`
	} `json:"properties"`
}

func (z azZone) toZone() Zone {
	n := z.Properties.NumberOf
	return Zone{ID: z.Name, Name: NormalizeDomain(z.Name), Provider: ProviderAzure, RecordCount: &n, NameServers: normalizeAll(z.Properties.NameServers), AccountID: ""}
}

// ListZones implements Provider.
func (a *Azure) ListZones(ctx context.Context) ([]Zone, error) {
	var out []Zone
	path := a.zonesPath() + "?api-version=" + azureAPIVersion
	for path != "" {
		var resp struct {
			Value    []azZone `json:"value"`
			NextLink string   `json:"nextLink"`
		}
		if err := a.rc.do(ctx, http.MethodGet, path, nil, &resp); err != nil {
			return nil, err
		}
		for _, z := range resp.Value {
			out = append(out, z.toZone())
		}
		path = ""
		if resp.NextLink != "" {
			// Only follow links on the ARM host so a hostile answer cannot redirect the bearer token.
			if u, err := url.Parse(resp.NextLink); err == nil {
				path = u.RequestURI()
			}
		}
	}
	return out, nil
}

// GetZone implements Provider.
func (a *Azure) GetZone(ctx context.Context, id string) (Zone, error) {
	var z azZone
	if err := a.rc.do(ctx, http.MethodGet, a.zonesPath()+"/"+url.PathEscape(id)+"?api-version="+azureAPIVersion, nil, &z); err != nil {
		if isNotFound(err) {
			return Zone{}, ErrZoneNotFound
		}
		return Zone{}, err
	}
	return z.toZone(), nil
}

// CreateZone implements Provider.
func (a *Azure) CreateZone(ctx context.Context, name string) (Zone, error) {
	var z azZone
	body := map[string]any{"location": "global", "properties": map[string]string{"zoneType": "Public"}}
	if err := a.rc.do(ctx, http.MethodPut, a.zonesPath()+"/"+url.PathEscape(NormalizeDomain(name))+"?api-version="+azureAPIVersion, body, &z); err != nil {
		return Zone{}, err
	}
	return z.toZone(), nil
}

// DeleteZone implements Provider.
func (a *Azure) DeleteZone(ctx context.Context, id string) error {
	return a.rc.do(ctx, http.MethodDelete, a.zonesPath()+"/"+url.PathEscape(id)+"?api-version="+azureAPIVersion, nil, nil)
}

type azProps struct {
	TTL int `json:"TTL"`
	A   []struct {
		IPv4 string `json:"ipv4Address"`
	} `json:"ARecords,omitempty"`
	AAAA []struct {
		IPv6 string `json:"ipv6Address"`
	} `json:"AAAARecords,omitempty"`
	CNAME *struct {
		Name string `json:"cname"`
	} `json:"CNAMERecord,omitempty"`
	MX []struct {
		Pref int    `json:"preference"`
		Host string `json:"exchange"`
	} `json:"MXRecords,omitempty"`
	NS []struct {
		Host string `json:"nsdname"`
	} `json:"NSRecords,omitempty"`
	PTR []struct {
		Host string `json:"ptrdname"`
	} `json:"PTRRecords,omitempty"`
	TXT []struct {
		Value []string `json:"value"`
	} `json:"TXTRecords,omitempty"`
	SRV []struct {
		Priority int    `json:"priority"`
		Weight   int    `json:"weight"`
		Port     int    `json:"port"`
		Target   string `json:"target"`
	} `json:"SRVRecords,omitempty"`
	CAA []struct {
		Flags int    `json:"flags"`
		Tag   string `json:"tag"`
		Value string `json:"value"`
	} `json:"CAARecords,omitempty"`
}

func (p azProps) values(typ string) []string {
	var v []string
	switch typ {
	case "A":
		for _, r := range p.A {
			v = append(v, r.IPv4)
		}
	case "AAAA":
		for _, r := range p.AAAA {
			v = append(v, r.IPv6)
		}
	case "CNAME":
		if p.CNAME != nil {
			v = append(v, NormalizeDomain(p.CNAME.Name))
		}
	case "MX":
		for _, r := range p.MX {
			v = append(v, fmt.Sprintf("%d %s", r.Pref, NormalizeDomain(r.Host)))
		}
	case "NS":
		for _, r := range p.NS {
			v = append(v, NormalizeDomain(r.Host))
		}
	case "PTR":
		for _, r := range p.PTR {
			v = append(v, NormalizeDomain(r.Host))
		}
	case "TXT":
		for _, r := range p.TXT {
			v = append(v, strings.Join(r.Value, ""))
		}
	case "SRV":
		for _, r := range p.SRV {
			v = append(v, fmt.Sprintf("%d %d %d %s", r.Priority, r.Weight, r.Port, NormalizeDomain(r.Target)))
		}
	case "CAA":
		for _, r := range p.CAA {
			v = append(v, fmt.Sprintf("%d %s %s", r.Flags, r.Tag, strconv.Quote(r.Value)))
		}
	}
	return v
}

func azProperties(rs RecordSet) (azProps, error) {
	p := azProps{TTL: rs.TTL}
	atoi := func(s string) int { n, _ := strconv.Atoi(s); return n }
	for _, val := range rs.Values {
		f := strings.Fields(val)
		switch rs.Type {
		case "A":
			p.A = append(p.A, struct {
				IPv4 string `json:"ipv4Address"`
			}{val})
		case "AAAA":
			p.AAAA = append(p.AAAA, struct {
				IPv6 string `json:"ipv6Address"`
			}{val})
		case "CNAME":
			p.CNAME = &struct {
				Name string `json:"cname"`
			}{val}
		case "NS":
			p.NS = append(p.NS, struct {
				Host string `json:"nsdname"`
			}{val})
		case "PTR":
			p.PTR = append(p.PTR, struct {
				Host string `json:"ptrdname"`
			}{val})
		case "TXT":
			p.TXT = append(p.TXT, struct {
				Value []string `json:"value"`
			}{splitTXT(val)})
		case "MX":
			if len(f) != 2 {
				return p, fmt.Errorf("azure: bad MX value %q", val)
			}
			p.MX = append(p.MX, struct {
				Pref int    `json:"preference"`
				Host string `json:"exchange"`
			}{atoi(f[0]), f[1]})
		case "SRV":
			if len(f) != 4 {
				return p, fmt.Errorf("azure: bad SRV value %q", val)
			}
			p.SRV = append(p.SRV, struct {
				Priority int    `json:"priority"`
				Weight   int    `json:"weight"`
				Port     int    `json:"port"`
				Target   string `json:"target"`
			}{atoi(f[0]), atoi(f[1]), atoi(f[2]), f[3]})
		case "CAA":
			if len(f) < 3 {
				return p, fmt.Errorf("azure: bad CAA value %q", val)
			}
			v := strings.TrimSpace(strings.SplitN(val, f[1], 2)[1])
			if uq, err := strconv.Unquote(v); err == nil {
				v = uq
			}
			p.CAA = append(p.CAA, struct {
				Flags int    `json:"flags"`
				Tag   string `json:"tag"`
				Value string `json:"value"`
			}{atoi(f[0]), f[1], v})
		default:
			return p, fmt.Errorf("azure: record type %s is not supported", rs.Type)
		}
	}
	return p, nil
}

// ListRecordSets implements Provider. SOA sets are skipped, Azure manages them.
func (a *Azure) ListRecordSets(ctx context.Context, zone Zone) ([]RecordSet, error) {
	var out []RecordSet
	path := a.zonesPath() + "/" + url.PathEscape(zone.ID) + "/recordsets?api-version=" + azureAPIVersion
	for path != "" {
		var resp struct {
			Value []struct {
				Name       string  `json:"name"`
				Type       string  `json:"type"`
				Properties azProps `json:"properties"`
			} `json:"value"`
			NextLink string `json:"nextLink"`
		}
		if err := a.rc.do(ctx, http.MethodGet, path, nil, &resp); err != nil {
			return nil, err
		}
		for _, r := range resp.Value {
			typ := r.Type[strings.LastIndex(r.Type, "/")+1:]
			if typ == "SOA" {
				continue
			}
			out = append(out, RecordSet{Name: RelativeName(r.Name, zone.Name), Type: typ, TTL: r.Properties.TTL, Values: r.Properties.values(typ), Routing: RoutingSimple})
		}
		path = ""
		if resp.NextLink != "" {
			if u, err := url.Parse(resp.NextLink); err == nil {
				path = u.RequestURI()
			}
		}
	}
	SortSets(out)
	return out, nil
}

func (a *Azure) setPath(zone Zone, name, typ string) string {
	return a.zonesPath() + "/" + url.PathEscape(zone.ID) + "/" + url.PathEscape(typ) + "/" + url.PathEscape(name) + "?api-version=" + azureAPIVersion
}

// UpsertRecordSet implements Provider (PUT replaces the whole set).
func (a *Azure) UpsertRecordSet(ctx context.Context, zone Zone, rs RecordSet) error {
	props, err := azProperties(rs)
	if err != nil {
		return err
	}
	return a.rc.do(ctx, http.MethodPut, a.setPath(zone, rs.Name, rs.Type), map[string]any{"properties": props}, nil)
}

// DeleteRecordSet implements Provider.
func (a *Azure) DeleteRecordSet(ctx context.Context, zone Zone, k Key) error {
	err := a.rc.do(ctx, http.MethodDelete, a.setPath(zone, k.Name, k.Type), nil, nil)
	if isNotFound(err) {
		return nil
	}
	return err
}
