package provision

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

const (
	digitaloceanListPageSize = 200
	// maxListPages bounds a runaway pagination loop on a misbehaving server.
	maxListPages = 100
)

type doListedDroplet struct {
	ID        int64    `json:"id"`
	Name      string   `json:"name"`
	Status    string   `json:"status"`
	CreatedAt string   `json:"created_at"`
	SizeSlug  string   `json:"size_slug"`
	Tags      []string `json:"tags"`
	Region    struct {
		Slug string `json:"slug"`
	} `json:"region"`
	Size struct {
		PriceMonthly float64 `json:"price_monthly"`
	} `json:"size"`
	Networks struct {
		V4 []struct {
			IPAddress string `json:"ip_address"`
			Type      string `json:"type"`
		} `json:"v4"`
	} `json:"networks"`
}

type doListedDropletsResponse struct {
	Droplets []doListedDroplet `json:"droplets"`
	doLinks
}

// ListResources calls GET /v2/droplets across every page and prices each
// droplet from the sizes list. A sizes failure only drops prices, a droplets
// failure is returned so the caller can map a 403 to the missing scope.
func (d *DigitalOcean) ListResources(ctx context.Context) ([]ProviderResource, error) {
	var droplets []doListedDroplet
	path := fmt.Sprintf("/droplets?per_page=%d", digitaloceanListPageSize)
	for pages := 0; path != "" && pages < maxListPages; pages++ {
		var out doListedDropletsResponse
		if err := d.client.do(ctx, http.MethodGet, path, nil, &out); err != nil {
			return nil, fmt.Errorf("provision: digitalocean list droplets: %w", err)
		}
		droplets = append(droplets, out.Droplets...)
		path = d.client.relativePath(out.Links.Pages.Next)
	}
	prices := d.sizePrices(ctx)
	res := make([]ProviderResource, 0, len(droplets))
	for _, dr := range droplets {
		r := ProviderResource{
			ID: fmt.Sprintf("%d", dr.ID), Name: dr.Name, Region: dr.Region.Slug, Size: dr.SizeSlug,
			Status: dr.Status, Tags: dr.Tags, Currency: "USD",
		}
		if r.Tags == nil {
			r.Tags = []string{}
		}
		if t, err := time.Parse(time.RFC3339, dr.CreatedAt); err == nil {
			r.CreatedAt = t.UTC()
		}
		for _, n := range dr.Networks.V4 {
			if n.Type == "public" {
				r.PublicIPv4 = n.IPAddress
				break
			}
		}
		if p, ok := prices[dr.SizeSlug]; ok {
			r.EstimatedMonthlyPrice = &p
		} else if dr.Size.PriceMonthly > 0 {
			p := dr.Size.PriceMonthly
			r.EstimatedMonthlyPrice = &p
		}
		res = append(res, r)
	}
	return res, nil
}

// sizePrices maps size slug to its monthly list price. Best effort: an
// error leaves the map empty and prices fall back to the droplet's own size.
func (d *DigitalOcean) sizePrices(ctx context.Context) map[string]float64 {
	prices := map[string]float64{}
	path := fmt.Sprintf("/sizes?per_page=%d", digitaloceanListPageSize)
	for pages := 0; path != "" && pages < maxListPages; pages++ {
		var out doSizesResponse
		if err := d.client.do(ctx, http.MethodGet, path, nil, &out); err != nil {
			return prices
		}
		for _, s := range out.Sizes {
			prices[s.Slug] = s.PriceMonthly
		}
		path = d.client.relativePath(out.Links.Pages.Next)
	}
	return prices
}
