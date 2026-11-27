package dockerguard

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/envredact"
)

// maxFilteredBody caps a list or inspect body the logs profile decodes.
const maxFilteredBody = 16 << 20

type respFilterKind int

const (
	respFilterNone respFilterKind = iota
	respFilterList
	respFilterInspect
	respFilterEvents
)

type respFilterKey struct{}

func withRespFilter(ctx context.Context, k respFilterKind) context.Context {
	return context.WithValue(ctx, respFilterKey{}, k)
}

func respFilterOf(ctx context.Context) respFilterKind {
	k, _ := ctx.Value(respFilterKey{}).(respFilterKind)
	return k
}

// logsFilterFor picks the response filter for a logs-profile path.
func logsFilterFor(pattern string) respFilterKind {
	switch pattern {
	case "/containers/json":
		return respFilterList
	case "/containers/*/json":
		return respFilterInspect
	case "/events":
		return respFilterEvents
	}
	return respFilterNone
}

// filterResponse rewrites a successful logs-profile response so it never
// carries environment, host paths or secret-looking labels.
func filterResponse(resp *http.Response) error {
	kind := respFilterOf(resp.Request.Context())
	if kind == respFilterNone || resp.StatusCode != http.StatusOK {
		return nil
	}
	if kind == respFilterEvents {
		resp.Body = newEventFilter(resp.Body)
		resp.ContentLength = -1
		resp.Header.Del("Content-Length")
		return nil
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxFilteredBody+1))
	_ = resp.Body.Close()
	if err != nil || len(raw) > maxFilteredBody {
		return errors.New("docker guard: response too large to filter")
	}
	out, err := FilterJSON(kind == respFilterList, raw)
	if err != nil {
		return err
	}
	resp.Body = io.NopCloser(bytes.NewReader(out))
	resp.ContentLength = int64(len(out))
	resp.Header.Set("Content-Length", strconv.Itoa(len(out)))
	return nil
}

// FilterJSON strips one inspect object, or each element of a list, down to
// what a log viewer needs. Unknown fields pass through untouched.
func FilterJSON(list bool, raw []byte) ([]byte, error) {
	if !list {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(raw, &obj); err != nil {
			return nil, err
		}
		stripInspect(obj)
		return json.Marshal(obj)
	}
	var items []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, err
	}
	for _, it := range items {
		stripListItem(it)
	}
	return json.Marshal(items)
}

func stripListItem(it map[string]json.RawMessage) {
	it["Labels"] = cleanLabels(it["Labels"])
	if m, ok := it["Mounts"]; ok {
		it["Mounts"] = reduceMounts(m)
	}
	delete(it, "HostConfig")
}

func stripInspect(obj map[string]json.RawMessage) {
	for _, k := range []string{"ResolvConfPath", "HostnamePath", "HostsPath", "LogPath", "GraphDriver", "AppArmorProfile", "ExecIDs"} {
		delete(obj, k)
	}
	if m, ok := obj["Mounts"]; ok {
		obj["Mounts"] = reduceMounts(m)
	}
	obj["Config"] = editObject(obj["Config"], func(c map[string]json.RawMessage) {
		c["Env"] = json.RawMessage("[]")
		c["Labels"] = cleanLabels(c["Labels"])
		delete(c, "Healthcheck")
	})
	obj["HostConfig"] = editObject(obj["HostConfig"], func(h map[string]json.RawMessage) {
		delete(h, "Binds")
		if m, ok := h["Mounts"]; ok {
			h["Mounts"] = reduceHostMounts(m)
		}
	})
}

func editObject(raw json.RawMessage, fn func(map[string]json.RawMessage)) json.RawMessage {
	var m map[string]json.RawMessage
	if len(raw) == 0 || json.Unmarshal(raw, &m) != nil || m == nil {
		return json.RawMessage("{}")
	}
	fn(m)
	out, err := json.Marshal(m)
	if err != nil {
		return json.RawMessage("{}")
	}
	return out
}

// cleanLabels drops labels whose key or value looks like a secret. Our own
// reserved labels stay, they carry no operator data.
func cleanLabels(raw json.RawMessage) json.RawMessage {
	var labels map[string]string
	if len(raw) == 0 || json.Unmarshal(raw, &labels) != nil {
		return json.RawMessage("{}")
	}
	out := make(map[string]string, len(labels))
	for k, v := range labels {
		if !strings.HasPrefix(k, reservedLabelPrefix) && (envredact.SensitiveName(k) || envredact.SensitiveValue(k, v)) {
			continue
		}
		out[k] = v
	}
	b, _ := json.Marshal(out)
	return b
}

// keepFields reduces each object in a JSON array to the listed keys.
func keepFields(raw json.RawMessage, keys ...string) json.RawMessage {
	var in []map[string]json.RawMessage
	if json.Unmarshal(raw, &in) != nil {
		return json.RawMessage("[]")
	}
	out := make([]map[string]json.RawMessage, 0, len(in))
	for _, m := range in {
		keep := map[string]json.RawMessage{}
		for _, k := range keys {
			if v, ok := m[k]; ok {
				keep[k] = v
			}
		}
		out = append(out, keep)
	}
	b, _ := json.Marshal(out)
	return b
}

// reduceMounts keeps type, volume name, destination and mode; host paths go.
func reduceMounts(raw json.RawMessage) json.RawMessage {
	return keepFields(raw, "Type", "Name", "Destination", "Mode", "RW")
}

func reduceHostMounts(raw json.RawMessage) json.RawMessage {
	return keepFields(raw, "Type", "Target", "ReadOnly")
}

// newEventFilter re-encodes the event stream: exec events carry command
// lines so they are dropped, and secret-looking attributes are removed.
func newEventFilter(src io.ReadCloser) io.ReadCloser {
	pr, pw := io.Pipe()
	go func() {
		defer func() { _ = src.Close() }()
		dec := json.NewDecoder(src)
		enc := json.NewEncoder(pw)
		for {
			var ev map[string]json.RawMessage
			if err := dec.Decode(&ev); err != nil {
				_ = pw.CloseWithError(err)
				return
			}
			if !filterEvent(ev) {
				continue
			}
			if err := enc.Encode(ev); err != nil {
				return
			}
		}
	}()
	return pr
}

func filterEvent(ev map[string]json.RawMessage) bool {
	var action string
	_ = json.Unmarshal(ev["Action"], &action)
	var status string
	_ = json.Unmarshal(ev["status"], &status)
	if strings.HasPrefix(action, "exec_") || strings.HasPrefix(status, "exec_") {
		return false
	}
	ev["Actor"] = editObject(ev["Actor"], func(a map[string]json.RawMessage) {
		a["Attributes"] = cleanLabels(a["Attributes"])
	})
	return true
}
