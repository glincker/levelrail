package dnszones

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// HTTPError is a non-2xx answer from a token-authenticated DNS API.
type HTTPError struct {
	Provider string
	Status   int
	Message  string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("%s: http %d: %s", e.Provider, e.Status, e.Message)
}

// restClient is the small JSON client shared by the DigitalOcean, Hetzner and Azure providers.
type restClient struct {
	provider string
	base     string
	header   func(*http.Request)
	http     *http.Client
}

func (c *restClient) client() *http.Client {
	if c.http != nil {
		return c.http
	}
	return &http.Client{Timeout: 20 * time.Second}
}

// do sends body as JSON and decodes a JSON answer into out. A 404 is returned as ErrZoneNotFound only by callers that ask for it.
func (c *restClient) do(ctx context.Context, method, path string, body, out any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("%s: encode body: %w", c.provider, err)
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rdr)
	if err != nil {
		return fmt.Errorf("%s: build request: %w", c.provider, err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.header != nil {
		c.header(req)
	}
	resp, err := c.client().Do(req)
	if err != nil {
		return fmt.Errorf("%s: %s %s: %w", c.provider, method, strings.SplitN(path, "?", 2)[0], err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if resp.StatusCode >= 300 {
		return &HTTPError{Provider: c.provider, Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	if out != nil && len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("%s: decode response: %w", c.provider, err)
		}
	}
	return nil
}

// errorMessage pulls a short message out of the common error body shapes.
func errorMessage(raw []byte) string {
	var m struct {
		Message string `json:"message"`
		Error   struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(raw, &m) == nil {
		if m.Message != "" {
			return m.Message
		}
		if m.Error.Message != "" {
			return m.Error.Message
		}
	}
	s := strings.TrimSpace(string(raw))
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

func isNotFound(err error) bool {
	var he *HTTPError
	return errors.As(err, &he) && he.Status == http.StatusNotFound
}

// flatRecord is one provider record in canonical form, used by the shared diff upsert.
type flatRecord struct {
	ID    string
	Name  string
	Type  string
	TTL   int
	Value string
}

// flatBackend is implemented by providers that store one record per value (DigitalOcean, Hetzner).
type flatBackend interface {
	list(ctx context.Context, zone Zone) ([]flatRecord, error)
	create(ctx context.Context, zone Zone, rs RecordSet, value string) error
	update(ctx context.Context, zone Zone, id string, rs RecordSet, value string) error
	remove(ctx context.Context, zone Zone, id string) error
}

func groupFlat(recs []flatRecord) []RecordSet {
	var out []RecordSet
	index := map[Key]int{}
	for _, r := range recs {
		k := Key{Name: r.Name, Type: r.Type}
		if i, ok := index[k]; ok {
			out[i].Values = append(out[i].Values, r.Value)
			continue
		}
		index[k] = len(out)
		out = append(out, RecordSet{Name: r.Name, Type: r.Type, TTL: r.TTL, Values: []string{r.Value}, Routing: RoutingSimple})
	}
	SortSets(out)
	return out
}

// upsertFlat keeps wanted values, creates missing ones, then deletes the rest so a live value is never absent.
func upsertFlat(ctx context.Context, b flatBackend, zone Zone, rs RecordSet) error {
	all, err := b.list(ctx, zone)
	if err != nil {
		return err
	}
	var existing []flatRecord
	for _, r := range all {
		if r.Name == rs.Name && r.Type == rs.Type {
			existing = append(existing, r)
		}
	}
	used := map[string]bool{}
	for _, v := range rs.Values {
		found := -1
		for i, e := range existing {
			if e.Value == v && !used[e.ID] {
				found = i
				break
			}
		}
		if found < 0 {
			if err := b.create(ctx, zone, rs, v); err != nil {
				return err
			}
			continue
		}
		e := existing[found]
		used[e.ID] = true
		if e.TTL != rs.TTL {
			if err := b.update(ctx, zone, e.ID, rs, v); err != nil {
				return err
			}
		}
	}
	for _, e := range existing {
		if !used[e.ID] {
			if err := b.remove(ctx, zone, e.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

func deleteFlat(ctx context.Context, b flatBackend, zone Zone, k Key) error {
	all, err := b.list(ctx, zone)
	if err != nil {
		return err
	}
	for _, r := range all {
		if r.Name == k.Name && r.Type == k.Type {
			if err := b.remove(ctx, zone, r.ID); err != nil {
				return err
			}
		}
	}
	return nil
}
