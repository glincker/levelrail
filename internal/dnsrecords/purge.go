package dnsrecords

import (
	"context"
	"net/http"
	"strings"
)

// cloudflarePurgeBatch is the documented cap on hostnames per purge call.
const cloudflarePurgeBatch = 30

// PurgeHosts clears Cloudflare's edge cache for every URL under each host in
// zone. It needs a token with the Cache Purge permission.
func (c *CloudflareAPI) PurgeHosts(ctx context.Context, zone string, hosts []string) error {
	if len(hosts) == 0 {
		return nil
	}
	z, ok, err := c.zoneByName(ctx, strings.TrimSuffix(zone, "."))
	if err != nil {
		return err
	}
	if !ok {
		return ErrZoneNotFound
	}
	for len(hosts) > 0 {
		n := min(len(hosts), cloudflarePurgeBatch)
		body := map[string][]string{"hosts": hosts[:n]}
		if err := c.do(ctx, http.MethodPost, "/zones/"+z.ID+"/purge_cache", body, nil); err != nil {
			return err
		}
		hosts = hosts[n:]
	}
	return nil
}

// SSLMode reads the zone's SSL/TLS encryption mode: off, flexible, full or
// strict. It needs a token with Zone Settings read.
func (c *CloudflareAPI) SSLMode(ctx context.Context, zone string) (string, error) {
	z, ok, err := c.zoneByName(ctx, strings.TrimSuffix(zone, "."))
	if err != nil {
		return "", err
	}
	if !ok {
		return "", ErrZoneNotFound
	}
	var setting struct {
		Value string `json:"value"`
	}
	if err := c.do(ctx, http.MethodGet, "/zones/"+z.ID+"/settings/ssl", nil, &setting); err != nil {
		return "", err
	}
	return setting.Value, nil
}
