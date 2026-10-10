package dnszones

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

type cfData struct {
	Priority *int   `json:"priority,omitempty"`
	Weight   *int   `json:"weight,omitempty"`
	Port     *int   `json:"port,omitempty"`
	Target   string `json:"target,omitempty"`
	Flags    *int   `json:"flags,omitempty"`
	Tag      string `json:"tag,omitempty"`
	Value    string `json:"value,omitempty"`
}

type cfRecord struct {
	ID       string  `json:"id,omitempty"`
	Type     string  `json:"type"`
	Name     string  `json:"name"`
	Content  string  `json:"content,omitempty"`
	Priority *int    `json:"priority,omitempty"`
	Proxied  *bool   `json:"proxied,omitempty"`
	TTL      int     `json:"ttl"`
	Data     *cfData `json:"data,omitempty"`
}

func intp(v int) *int { return &v }

// value renders a Cloudflare record in canonical value form.
func (r cfRecord) value() string {
	d := r.Data
	switch r.Type {
	case "MX":
		p := 0
		if r.Priority != nil {
			p = *r.Priority
		}
		return fmt.Sprintf("%d %s", p, NormalizeDomain(r.Content))
	case "SRV":
		if d != nil && d.Priority != nil && d.Weight != nil && d.Port != nil {
			return fmt.Sprintf("%d %d %d %s", *d.Priority, *d.Weight, *d.Port, NormalizeDomain(d.Target))
		}
	case "CAA":
		if d != nil && d.Flags != nil {
			return fmt.Sprintf("%d %s %s", *d.Flags, d.Tag, strconv.Quote(d.Value))
		}
	case "TXT":
		return unquoteTXT(r.Content)
	case "CNAME", "NS":
		return NormalizeDomain(r.Content)
	}
	return r.Content
}

// toCFRecord builds the API body for one value of rs.
func toCFRecord(rs RecordSet, zone Zone, value string) (cfRecord, error) {
	rec := cfRecord{Type: rs.Type, Name: FQDN(rs.Name, zone.Name), TTL: rs.TTL}
	if rs.Type == "A" || rs.Type == "AAAA" || rs.Type == "CNAME" {
		p := rs.Proxied
		rec.Proxied = &p
	}
	f := strings.Fields(value)
	atoi := func(s string) int { n, _ := strconv.Atoi(s); return n }
	switch rs.Type {
	case "MX":
		if len(f) != 2 {
			return rec, fmt.Errorf("cloudflare: bad MX value %q", value)
		}
		rec.Priority, rec.Content = intp(atoi(f[0])), f[1]
	case "SRV":
		if len(f) != 4 {
			return rec, fmt.Errorf("cloudflare: bad SRV value %q", value)
		}
		rec.Data = &cfData{Priority: intp(atoi(f[0])), Weight: intp(atoi(f[1])), Port: intp(atoi(f[2])), Target: f[3]}
	case "CAA":
		if len(f) < 3 {
			return rec, fmt.Errorf("cloudflare: bad CAA value %q", value)
		}
		v := strings.TrimSpace(strings.SplitN(value, f[1], 2)[1])
		if uq, err := strconv.Unquote(v); err == nil {
			v = uq
		}
		rec.Data = &cfData{Flags: intp(atoi(f[0])), Tag: f[1], Value: v}
	case "TXT":
		rec.Content = quoteTXT(value)
	default:
		rec.Content = value
	}
	return rec, nil
}

func (c *Cloudflare) records(ctx context.Context, zone Zone, query string) ([]cfRecord, error) {
	return paginate[cfRecord](ctx, c, "/zones/"+url.PathEscape(zone.ID)+"/dns_records?per_page=100"+query)
}

// ListRecordSets implements Provider (GET /zones/{id}/dns_records), grouping
// Cloudflare's individual records by name and type.
func (c *Cloudflare) ListRecordSets(ctx context.Context, zone Zone) ([]RecordSet, error) {
	recs, err := c.records(ctx, zone, "")
	if err != nil {
		return nil, err
	}
	var out []RecordSet
	index := map[Key]int{}
	for _, r := range recs {
		k := Key{Name: RelativeName(r.Name, zone.Name), Type: r.Type}
		if i, ok := index[k]; ok {
			out[i].Values = append(out[i].Values, r.value())
			continue
		}
		index[k] = len(out)
		rs := RecordSet{Name: k.Name, Type: r.Type, TTL: r.TTL, Values: []string{r.value()}, Routing: RoutingSimple}
		if r.Proxied != nil {
			rs.Proxied = *r.Proxied
		}
		out = append(out, rs)
	}
	SortSets(out)
	return out, nil
}

func (c *Cloudflare) setRecords(ctx context.Context, zone Zone, k Key) ([]cfRecord, error) {
	q := "&type=" + url.QueryEscape(k.Type) + "&name=" + url.QueryEscape(FQDN(k.Name, zone.Name))
	return c.records(ctx, zone, q)
}

// UpsertRecordSet implements Provider: keeps records whose value is wanted
// (patching TTL or proxied when they differ), creates missing ones, then
// deletes the rest, so a live value is never briefly absent.
func (c *Cloudflare) UpsertRecordSet(ctx context.Context, zone Zone, rs RecordSet) error {
	existing, err := c.setRecords(ctx, zone, rs.Key())
	if err != nil {
		return err
	}
	base := "/zones/" + url.PathEscape(zone.ID) + "/dns_records"
	var keep []string
	for _, v := range rs.Values {
		body, err := toCFRecord(rs, zone, v)
		if err != nil {
			return err
		}
		i := slices.IndexFunc(existing, func(e cfRecord) bool { return e.value() == v && !slices.Contains(keep, e.ID) })
		if i < 0 {
			if _, err := c.call(ctx, http.MethodPost, base, body, nil); err != nil {
				return err
			}
			continue
		}
		e := existing[i]
		keep = append(keep, e.ID)
		if e.TTL != rs.TTL || (body.Proxied != nil && (e.Proxied == nil || *e.Proxied != *body.Proxied)) {
			if _, err := c.call(ctx, http.MethodPatch, base+"/"+url.PathEscape(e.ID), body, nil); err != nil {
				return err
			}
		}
	}
	for _, e := range existing {
		if slices.Contains(keep, e.ID) {
			continue
		}
		if _, err := c.call(ctx, http.MethodDelete, base+"/"+url.PathEscape(e.ID), nil, nil); err != nil {
			return err
		}
	}
	return nil
}

// DeleteRecordSet implements Provider: deletes every record of name and type.
func (c *Cloudflare) DeleteRecordSet(ctx context.Context, zone Zone, k Key) error {
	existing, err := c.setRecords(ctx, zone, k)
	if err != nil {
		return err
	}
	base := "/zones/" + url.PathEscape(zone.ID) + "/dns_records/"
	for _, e := range existing {
		if _, err := c.call(ctx, http.MethodDelete, base+url.PathEscape(e.ID), nil, nil); err != nil {
			return err
		}
	}
	return nil
}
