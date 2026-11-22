package dnszones

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ProviderNamecheap names the Namecheap DNS provider.
const ProviderNamecheap = "namecheap"

// NamecheapBaseURL is the Namecheap XML API endpoint.
const NamecheapBaseURL = "https://api.namecheap.com/xml.response"

const (
	namecheapDefaultTTL = 1800
	namecheapMinTTL     = 60
	namecheapPageSize   = 100
)

// Namecheap implements Provider over the Namecheap XML API. Its setHosts call replaces every
// record of a domain, so each change is read, merged and written back as a whole.
type Namecheap struct {
	user, key, clientIP string
	base                string
	http                *http.Client
	mu                  sync.Mutex
}

// NewNamecheap parses credential as "apiuser:apikey:clientip". The client IP must be
// whitelisted in the Namecheap account.
func NewNamecheap(credential string) (*Namecheap, error) {
	parts := strings.Split(strings.TrimSpace(credential), ":")
	if len(parts) != 3 {
		return nil, errors.New("namecheap: credential must be apiuser:apikey:clientip")
	}
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
		if parts[i] == "" {
			return nil, errors.New("namecheap: credential must be apiuser:apikey:clientip")
		}
	}
	return &Namecheap{user: parts[0], key: parts[1], clientIP: parts[2], base: NamecheapBaseURL}, nil
}

// SetBaseURL points the client at another API endpoint (tests).
func (n *Namecheap) SetBaseURL(base string, c *http.Client) { n.base, n.http = base, c }

// Name implements Provider.
func (n *Namecheap) Name() string { return ProviderNamecheap }

// Capabilities implements Provider.
func (n *Namecheap) Capabilities() Capabilities { return Capabilities{} }

type ncHost struct {
	Name    string `xml:"Name,attr"`
	Type    string `xml:"Type,attr"`
	Address string `xml:"Address,attr"`
	MXPref  string `xml:"MXPref,attr"`
	TTL     string `xml:"TTL,attr"`
}

type ncResponse struct {
	Status string   `xml:"Status,attr"`
	Errors []string `xml:"Errors>Error"`
	Zones  []struct {
		Name string `xml:"Name,attr"`
	} `xml:"CommandResponse>DomainGetListResult>Domain"`
	HostsResult struct {
		OurDNS string   `xml:"IsUsingOurDNS,attr"`
		Hosts  []ncHost `xml:"host"`
	} `xml:"CommandResponse>DomainDNSGetHostsResult"`
	SetResult struct {
		Success string `xml:"IsSuccess,attr"`
	} `xml:"CommandResponse>DomainDNSSetHostsResult"`
}

// call posts the command as a form so the API key never appears in a URL or an error.
func (n *Namecheap) call(ctx context.Context, command string, extra url.Values) (*ncResponse, error) {
	form := url.Values{"ApiUser": {n.user}, "ApiKey": {n.key}, "UserName": {n.user}, "ClientIp": {n.clientIP}, "Command": {command}}
	for k, v := range extra {
		form[k] = v
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.base, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, errors.New("namecheap: build request")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	c := n.http
	if c == nil {
		c = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := c.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return nil, fmt.Errorf("namecheap: %s: %w", command, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if resp.StatusCode >= 300 {
		return nil, &HTTPError{Provider: ProviderNamecheap, Status: resp.StatusCode, Message: "request rejected"}
	}
	var out ncResponse
	if err := xml.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("namecheap: %s: unreadable answer", command)
	}
	if !strings.EqualFold(out.Status, "OK") {
		msg := strings.Join(out.Errors, "; ")
		if msg == "" {
			msg = "request failed"
		}
		return nil, fmt.Errorf("namecheap: %s: %s", command, msg)
	}
	return &out, nil
}

// ListZones implements Provider.
func (n *Namecheap) ListZones(ctx context.Context) ([]Zone, error) {
	var out []Zone
	for page := 1; page < 1000; page++ {
		r, err := n.call(ctx, "namecheap.domains.getList", url.Values{"PageSize": {strconv.Itoa(namecheapPageSize)}, "Page": {strconv.Itoa(page)}})
		if err != nil {
			return nil, err
		}
		for _, d := range r.Zones {
			name := NormalizeDomain(d.Name)
			out = append(out, Zone{ID: name, Name: name, Provider: ProviderNamecheap, Status: "ACTIVE"})
		}
		if len(r.Zones) < namecheapPageSize {
			break
		}
	}
	return out, nil
}

// GetZone implements Provider.
func (n *Namecheap) GetZone(ctx context.Context, id string) (Zone, error) {
	zones, err := n.ListZones(ctx)
	if err != nil {
		return Zone{}, err
	}
	return FindZone(zones, id)
}

// CreateZone implements Provider.
func (n *Namecheap) CreateZone(context.Context, string) (Zone, error) {
	return Zone{}, errors.New("namecheap: register the domain at Namecheap; zones cannot be created here")
}

// DeleteZone implements Provider.
func (n *Namecheap) DeleteZone(context.Context, string) error {
	return errors.New("namecheap: a domain's DNS zone cannot be deleted through the API")
}

// getHosts reads every record. It refuses a domain on other nameservers and an empty answer,
// because writing back after a bad read would wipe the zone.
func (n *Namecheap) getHosts(ctx context.Context, zone Zone) ([]ncHost, error) {
	sld, tld, ok := strings.Cut(NormalizeDomain(zone.Name), ".")
	if !ok || sld == "" || tld == "" {
		return nil, fmt.Errorf("namecheap: %q is not a registered domain", zone.Name)
	}
	r, err := n.call(ctx, "namecheap.domains.dns.getHosts", url.Values{"SLD": {sld}, "TLD": {tld}})
	if err != nil {
		return nil, err
	}
	if strings.EqualFold(r.HostsResult.OurDNS, "false") {
		return nil, errors.New("namecheap: the domain does not use Namecheap DNS, change its records where its nameservers point")
	}
	if len(r.HostsResult.Hosts) == 0 {
		return nil, errors.New("namecheap: refusing to change a domain whose current records could not be read or are empty")
	}
	return r.HostsResult.Hosts, nil
}

// setHosts writes the complete record list back.
func (n *Namecheap) setHosts(ctx context.Context, zone Zone, hosts []ncHost) error {
	sld, tld, _ := strings.Cut(NormalizeDomain(zone.Name), ".")
	form := url.Values{"SLD": {sld}, "TLD": {tld}}
	for i, h := range hosts {
		k := strconv.Itoa(i + 1)
		form.Set("HostName"+k, h.Name)
		form.Set("RecordType"+k, h.Type)
		form.Set("Address"+k, h.Address)
		if h.Type == "MX" {
			form.Set("MXPref"+k, h.MXPref)
			form.Set("EmailType", "MX")
		}
		form.Set("TTL"+k, h.TTL)
	}
	r, err := n.call(ctx, "namecheap.domains.dns.setHosts", form)
	if err != nil {
		return err
	}
	if strings.EqualFold(r.SetResult.Success, "false") {
		return errors.New("namecheap: setHosts was not applied")
	}
	return nil
}

// ListRecordSets implements Provider.
func (n *Namecheap) ListRecordSets(ctx context.Context, zone Zone) ([]RecordSet, error) {
	hosts, err := n.getHosts(ctx, zone)
	if err != nil {
		return nil, err
	}
	flat := make([]flatRecord, 0, len(hosts))
	for _, h := range hosts {
		ttl, _ := strconv.Atoi(h.TTL)
		v := h.Address
		if h.Type == "MX" {
			v = strings.TrimSpace(h.MXPref + " " + h.Address)
		}
		flat = append(flat, flatRecord{Name: h.Name, Type: h.Type, TTL: ttl, Value: v})
	}
	return groupFlat(flat), nil
}

// without drops every host of the given name and type.
func without(hosts []ncHost, name, typ string) []ncHost {
	out := make([]ncHost, 0, len(hosts))
	for _, h := range hosts {
		if strings.EqualFold(h.Name, name) && strings.EqualFold(h.Type, typ) {
			continue
		}
		out = append(out, h)
	}
	return out
}

// UpsertRecordSet implements Provider. Read-merge-write under a lock keeps other records intact.
func (n *Namecheap) UpsertRecordSet(ctx context.Context, zone Zone, rs RecordSet) error {
	if rs.Type == "SRV" {
		return fmt.Errorf("namecheap: SRV records are %w", ErrUnsupported)
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	hosts, err := n.getHosts(ctx, zone)
	if err != nil {
		return err
	}
	ttl := rs.TTL
	if ttl <= 0 {
		ttl = namecheapDefaultTTL
	}
	ttl = max(ttl, namecheapMinTTL)
	merged := without(hosts, rs.Name, rs.Type)
	for _, v := range rs.Values {
		h := ncHost{Name: rs.Name, Type: rs.Type, Address: v, TTL: strconv.Itoa(ttl)}
		if rs.Type == "MX" {
			pref, host, ok := strings.Cut(strings.TrimSpace(v), " ")
			if _, perr := strconv.Atoi(pref); !ok || perr != nil {
				return fmt.Errorf("namecheap: MX value %q needs a priority and a host", v)
			}
			h.MXPref, h.Address = pref, strings.TrimSpace(host)
		}
		merged = append(merged, h)
	}
	return n.setHosts(ctx, zone, merged)
}

// DeleteRecordSet implements Provider.
func (n *Namecheap) DeleteRecordSet(ctx context.Context, zone Zone, k Key) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	hosts, err := n.getHosts(ctx, zone)
	if err != nil {
		return err
	}
	merged := without(hosts, k.Name, k.Type)
	if len(merged) == len(hosts) {
		return nil
	}
	if len(merged) == 0 {
		return errors.New("namecheap: refusing to delete the last record of a domain")
	}
	return n.setHosts(ctx, zone, merged)
}
