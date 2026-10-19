// Command cloudflare-dns manages DNS records in Cloudflare for the control plane.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/plugin/pluginpb"
	"github.com/GLINCKER/levelrail/plugins/internal/pluginkit"
)

const (
	pluginName    = "cloudflare-dns"
	pluginVersion = "0.1.0"
	maxPages      = 20
	maxBody       = 1 << 20
)

var (
	apiBase = "https://api.cloudflare.com/client/v4"
	client  = &http.Client{Timeout: 12 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
)

type envelope struct {
	Success bool `json:"success"`
	Errors  []struct {
		Message string `json:"message"`
	} `json:"errors"`
	Result     json.RawMessage `json:"result"`
	ResultInfo struct {
		TotalPages int `json:"total_pages"`
	} `json:"result_info"`
}

type cfRecord struct {
	ID      string `json:"id,omitempty"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	TTL     int    `json:"ttl"`
	Proxied bool   `json:"proxied"`
}

func newPlugin() pluginkit.Plugin {
	return pluginkit.Plugin{
		Name: pluginName, Version: pluginVersion,
		ConfigSchema: `{"type":"object","additionalProperties":false}`,
		DNS: &pluginkit.DNSProvider{
			ListZones: listZones, ListRecords: listRecords, Upsert: upsert, Delete: remove, Verify: verify,
		},
	}
}

func fail(detail string, retryable bool) pluginkit.DNSOutcome {
	return pluginkit.DNSOutcome{Detail: detail, Retryable: retryable}
}

// call performs one API request and decodes the envelope. Every message it
// returns has the token removed.
func call(ctx context.Context, token, method, path string, body any) (envelope, *pluginkit.DNSOutcome) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return envelope{}, ptr(fail("encode request: "+pluginkit.Redact(err.Error(), token), false))
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, apiBase+path, rdr)
	if err != nil {
		return envelope{}, ptr(fail("build request: "+pluginkit.Redact(err.Error(), token), false))
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return envelope{}, ptr(fail("request failed: "+pluginkit.Redact(err.Error(), token), true))
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	_ = resp.Body.Close()
	switch code := resp.StatusCode; {
	case code == http.StatusUnauthorized || code == http.StatusForbidden:
		return envelope{}, ptr(fail(fmt.Sprintf("authentication failed (HTTP %d): check the Cloudflare token has Zone:DNS:Edit", code), false))
	case code == http.StatusTooManyRequests:
		return envelope{}, ptr(fail("rate limited by Cloudflare (HTTP 429)", true))
	case code >= 500:
		return envelope{}, ptr(fail(fmt.Sprintf("Cloudflare service error (HTTP %d)", code), true))
	}
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return envelope{}, ptr(fail(fmt.Sprintf("unreadable Cloudflare response (HTTP %d)", resp.StatusCode), false))
	}
	if !env.Success {
		msg := "Cloudflare rejected the request"
		if len(env.Errors) > 0 {
			msg += ": " + env.Errors[0].Message
		}
		return envelope{}, ptr(fail(pluginkit.Truncate(pluginkit.Redact(msg, token), 200), false))
	}
	return env, nil
}

func ptr(o pluginkit.DNSOutcome) *pluginkit.DNSOutcome { return &o }

func needToken(token string) *pluginkit.DNSOutcome {
	if strings.TrimSpace(token) == "" {
		return ptr(fail("a Cloudflare API token is required", false))
	}
	return nil
}

func zoneName(z string) string { return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(z)), ".") }

func fqdn(name, zone string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" || name == "@" {
		return zone
	}
	if strings.HasSuffix(name, "."+zone) || name == zone {
		return name
	}
	return name + "." + zone
}

func relative(full, zone string) string {
	full = strings.ToLower(full)
	if full == zone {
		return "@"
	}
	return strings.TrimSuffix(full, "."+zone)
}

func listZones(ctx context.Context, c pluginkit.DNSCall) ([]string, pluginkit.DNSOutcome) {
	if o := needToken(c.Secret); o != nil {
		return nil, *o
	}
	var names []string
	for page := 1; page <= maxPages; page++ {
		env, o := call(ctx, c.Secret, http.MethodGet, "/zones?per_page=50&page="+strconv.Itoa(page), nil)
		if o != nil {
			return nil, *o
		}
		var zs []struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(env.Result, &zs); err != nil {
			return nil, fail("unreadable zone list", false)
		}
		for _, z := range zs {
			names = append(names, z.Name+".")
		}
		if page >= env.ResultInfo.TotalPages {
			break
		}
	}
	return names, pluginkit.DNSOutcome{OK: true}
}

func zoneID(ctx context.Context, token, zone string) (string, *pluginkit.DNSOutcome) {
	env, o := call(ctx, token, http.MethodGet, "/zones?name="+url.QueryEscape(zoneName(zone)), nil)
	if o != nil {
		return "", o
	}
	var zs []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(env.Result, &zs); err != nil || len(zs) == 0 {
		return "", ptr(fail("zone "+zoneName(zone)+" was not found for this token", false))
	}
	return zs[0].ID, nil
}

func records(ctx context.Context, token, id, query string) ([]cfRecord, *pluginkit.DNSOutcome) {
	var out []cfRecord
	for page := 1; page <= maxPages; page++ {
		env, o := call(ctx, token, http.MethodGet, "/zones/"+id+"/dns_records?per_page=100&page="+strconv.Itoa(page)+query, nil)
		if o != nil {
			return nil, o
		}
		var rs []cfRecord
		if err := json.Unmarshal(env.Result, &rs); err != nil {
			return nil, ptr(fail("unreadable record list", false))
		}
		out = append(out, rs...)
		if page >= env.ResultInfo.TotalPages {
			break
		}
	}
	return out, nil
}

func listRecords(ctx context.Context, c pluginkit.DNSCall) ([]*pluginpb.DnsRecord, pluginkit.DNSOutcome) {
	if o := needToken(c.Secret); o != nil {
		return nil, *o
	}
	id, o := zoneID(ctx, c.Secret, c.Zone)
	if o != nil {
		return nil, *o
	}
	rs, o := records(ctx, c.Secret, id, "")
	if o != nil {
		return nil, *o
	}
	zone := zoneName(c.Zone)
	out := make([]*pluginpb.DnsRecord, 0, len(rs))
	for _, r := range rs {
		out = append(out, &pluginpb.DnsRecord{Name: relative(r.Name, zone), Type: r.Type, Value: r.Content, TtlSeconds: uint32(max(r.TTL, 0)), Proxied: r.Proxied}) //nolint:gosec // TTL is clamped non-negative
	}
	return out, pluginkit.DNSOutcome{OK: true}
}

func validRecord(r *pluginpb.DnsRecord) error {
	if r == nil || r.GetType() == "" || r.GetValue() == "" {
		return errors.New("a record with a type and a value is required")
	}
	return nil
}

func sameValue(a, b string) bool {
	return strings.EqualFold(strings.TrimSuffix(a, "."), strings.TrimSuffix(b, "."))
}

// matching keeps the records with the same name and type, and the same value when value is set.
func matching(rs []cfRecord, typ, name, value string) []cfRecord {
	var out []cfRecord
	for _, x := range rs {
		if strings.EqualFold(x.Name, name) && x.Type == typ && (value == "" || sameValue(x.Content, value)) {
			out = append(out, x)
		}
	}
	return out
}

func lookup(ctx context.Context, c pluginkit.DNSCall) (id, name string, rs []cfRecord, o *pluginkit.DNSOutcome) {
	if o := needToken(c.Secret); o != nil {
		return "", "", nil, o
	}
	if err := validRecord(c.Record); err != nil {
		return "", "", nil, ptr(fail(err.Error(), false))
	}
	name = fqdn(c.Record.GetName(), zoneName(c.Zone))
	id, o = zoneID(ctx, c.Secret, c.Zone)
	if o != nil {
		return "", "", nil, o
	}
	rs, o = records(ctx, c.Secret, id, "&type="+url.QueryEscape(c.Record.GetType())+"&name="+url.QueryEscape(name))
	return id, name, rs, o
}

func upsert(ctx context.Context, c pluginkit.DNSCall) pluginkit.DNSOutcome {
	id, name, rs, o := lookup(ctx, c)
	if o != nil {
		return *o
	}
	ttl := int(c.Record.GetTtlSeconds())
	if ttl == 0 {
		ttl = 1
	}
	body := cfRecord{Type: c.Record.GetType(), Name: name, Content: c.Record.GetValue(), TTL: ttl, Proxied: c.Record.GetProxied()}
	method, path := http.MethodPost, "/zones/"+id+"/dns_records"
	if cur := matching(rs, c.Record.GetType(), name, ""); len(cur) > 0 {
		method, path = http.MethodPut, path+"/"+cur[0].ID
	}
	if _, o := call(ctx, c.Secret, method, path, body); o != nil {
		return *o
	}
	return pluginkit.DNSOutcome{OK: true, Detail: "record saved"}
}

func remove(ctx context.Context, c pluginkit.DNSCall) pluginkit.DNSOutcome {
	id, name, rs, o := lookup(ctx, c)
	if o != nil {
		return *o
	}
	for _, r := range matching(rs, c.Record.GetType(), name, c.Record.GetValue()) {
		if _, o := call(ctx, c.Secret, http.MethodDelete, "/zones/"+id+"/dns_records/"+r.ID, nil); o != nil {
			return *o
		}
	}
	return pluginkit.DNSOutcome{OK: true, Detail: "record removed"}
}

func verify(ctx context.Context, c pluginkit.DNSCall) (bool, pluginkit.DNSOutcome) {
	_, name, rs, o := lookup(ctx, c)
	if o != nil {
		return false, *o
	}
	present := len(matching(rs, c.Record.GetType(), name, c.Record.GetValue())) > 0
	detail := "record is live"
	if !present {
		detail = "record was not found after the write"
	}
	return present, pluginkit.DNSOutcome{OK: true, Detail: detail}
}

func main() { pluginkit.Main(newPlugin()) }
