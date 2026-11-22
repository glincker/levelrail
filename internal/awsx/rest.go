package awsx

import (
	"context"
	"encoding/xml"
	"net/http"
)

// REST sends one REST/XML call. A non empty host overrides the regional endpoint for global
// services; signing still uses region. A nil body sends no payload.
func (c *Client) REST(ctx context.Context, svc Service, host, region, method, path string, body []byte, out any) error {
	a := attempt{svc: svc, region: region, op: method + " " + path, method: method, path: path, endpoint: host, body: body}
	if len(body) > 0 {
		a.contentType = "application/xml"
	}
	raw, err := c.do(ctx, a, decodeXMLError)
	if err != nil || out == nil {
		return err
	}
	return xml.Unmarshal(raw, out)
}

// RESTGet is REST with GET and no body.
func (c *Client) RESTGet(ctx context.Context, svc Service, host, region, path string, out any) error {
	return c.REST(ctx, svc, host, region, http.MethodGet, path, nil, out)
}
