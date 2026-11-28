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

	"github.com/glincker/levelrail/internal/envredact"
)

// maxFilteredBody caps a list or inspect body the logs profile decodes.
const maxFilteredBody = 16 << 20

type respFilterKind int

const (
	respFilterNone respFilterKind = iota
	respFilterList
	respFilterInspect
	respFilterEvents
	respFilterInfo
	respFilterInfoMin
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
	case "/info":
		return respFilterInfo
	}
	return respFilterNone
}

// filterResponse rewrites a successful response so it never carries
// environment, command lines, host paths or secret-looking labels.
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
	var out []byte
	switch kind {
	case respFilterInfo, respFilterInfoMin:
		out, err = filterInfo(raw, kind == respFilterInfo)
	default:
		out, err = FilterJSON(kind == respFilterList, raw)
	}
	if err != nil {
		return err
	}
	resp.Body = io.NopCloser(bytes.NewReader(out))
	resp.ContentLength = int64(len(out))
	resp.Header.Set("Content-Length", strconv.Itoa(len(out)))
	return nil
}

// FilterJSON rebuilds one inspect object, or each element of a list, from
// explicit allowlists. A field not named here never reaches the client.
func FilterJSON(list bool, raw []byte) ([]byte, error) {
	if !list {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(raw, &obj); err != nil {
			return nil, err
		}
		return json.Marshal(buildInspect(obj))
	}
	var items []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, err
	}
	out := make([]map[string]json.RawMessage, 0, len(items))
	for _, it := range items {
		out = append(out, buildListItem(it))
	}
	return json.Marshal(out)
}

// pick copies the named keys of src into a new object.
func pick(src map[string]json.RawMessage, keys ...string) map[string]json.RawMessage {
	out := map[string]json.RawMessage{}
	for _, k := range keys {
		if v, ok := src[k]; ok {
			out[k] = v
		}
	}
	return out
}

func asObject(raw json.RawMessage) map[string]json.RawMessage {
	var m map[string]json.RawMessage
	if len(raw) == 0 || json.Unmarshal(raw, &m) != nil || m == nil {
		return map[string]json.RawMessage{}
	}
	return m
}

func mustRaw(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage("{}")
	}
	return b
}

func buildListItem(it map[string]json.RawMessage) map[string]json.RawMessage {
	out := pick(it, "Id", "Names", "Created", "State", "Status")
	out["Image"] = cleanImage(it["Image"])
	out["Labels"] = cleanLabels(it["Labels"])
	out["HostConfig"] = mustRaw(pick(asObject(it["HostConfig"]), "NetworkMode"))
	out["NetworkSettings"] = networkNames(asObject(it["NetworkSettings"])["Networks"])
	return out
}

func buildInspect(obj map[string]json.RawMessage) map[string]json.RawMessage {
	out := pick(obj, "Id", "Name", "Created", "RestartCount")
	rawState := asObject(obj["State"])
	state := pick(rawState, "Status", "Running", "Paused", "Restarting", "OOMKilled", "Dead", "ExitCode", "StartedAt", "FinishedAt")
	if h := asObject(rawState["Health"]); len(h) > 0 {
		state["Health"] = mustRaw(pick(h, "Status"))
	}
	out["State"] = mustRaw(state)
	rawCfg := asObject(obj["Config"])
	cfg := pick(rawCfg, "Tty")
	cfg["Image"] = cleanImage(rawCfg["Image"])
	cfg["Labels"] = cleanLabels(rawCfg["Labels"])
	out["Config"] = mustRaw(cfg)
	out["HostConfig"] = mustRaw(pick(asObject(obj["HostConfig"]), "RestartPolicy", "NetworkMode", "Memory", "NanoCpus"))
	out["NetworkSettings"] = networkNames(asObject(obj["NetworkSettings"])["Networks"])
	return out
}

// networkNames keeps each attached network's name and drops IPs and gateways.
func networkNames(raw json.RawMessage) json.RawMessage {
	nets := map[string]json.RawMessage{}
	for name := range asObject(raw) {
		nets[name] = json.RawMessage("{}")
	}
	return mustRaw(map[string]json.RawMessage{"Networks": mustRaw(nets)})
}

// cleanImage drops registry credentials a pasted image reference may carry.
func cleanImage(raw json.RawMessage) json.RawMessage {
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return json.RawMessage(`""`)
	}
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if host, rest, found := strings.Cut(s, "/"); found {
		if at := strings.LastIndex(host, "@"); at >= 0 {
			s = host[at+1:] + "/" + rest
		}
	}
	return mustRaw(s)
}

// cleanLabels keeps only labels that are neither secret-looking nor host
// paths. Our own reserved labels stay, they carry no operator data.
func cleanLabels(raw json.RawMessage) json.RawMessage {
	var labels map[string]string
	if len(raw) == 0 || json.Unmarshal(raw, &labels) != nil {
		return json.RawMessage("{}")
	}
	out := make(map[string]string, len(labels))
	for k, v := range labels {
		if !strings.HasPrefix(k, reservedLabelPrefix) && dropLabel(k, v) {
			continue
		}
		out[k] = v
	}
	return mustRaw(out)
}

func dropLabel(k, v string) bool {
	up := strings.ToUpper(k)
	if strings.Contains(up, "AUTH") || strings.Contains(up, "USERS") {
		return true
	}
	lk := strings.ToLower(k)
	if strings.HasSuffix(lk, ".working_dir") || strings.HasSuffix(lk, ".config_files") {
		return true
	}
	return looksLikeHostPath(v) || envredact.SensitiveName(k) || envredact.SensitiveValue(k, v)
}

// looksLikeHostPath matches absolute unix and windows paths, also in lists.
func looksLikeHostPath(v string) bool {
	parts := strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == ';' || r == ' ' })
	for _, p := range parts {
		if strings.HasPrefix(p, "/") && strings.Count(p, "/") >= 2 {
			return true
		}
		if len(p) > 2 && p[1] == ':' && (p[2] == '\\' || p[2] == '/') {
			return true
		}
	}
	return false
}

// infoFields are the daemon versions and counts a viewer may read.
var infoFields = []string{"ServerVersion", "OperatingSystem", "OSType", "Architecture", "NCPU", "MemTotal", "Containers", "ContainersRunning", "ContainersPaused", "ContainersStopped", "Images"}

// filterInfo reduces /info to the allowlist; identity (ID, Name) is added
// only for the logs profile, whose viewers label hosts with it.
func filterInfo(raw []byte, withIdentity bool) ([]byte, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, err
	}
	out := pick(obj, infoFields...)
	if withIdentity {
		for k, v := range pick(obj, "ID", "Name") {
			out[k] = v
		}
	}
	return json.Marshal(out)
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
	actor := asObject(ev["Actor"])
	actor["Attributes"] = cleanLabels(actor["Attributes"])
	ev["Actor"] = mustRaw(actor)
	return true
}
