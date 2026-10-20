package api

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
)

var scimMemberFilterPath = regexp.MustCompile(`(?i)^members\[\s*value\s+eq\s+"([^"]+)"\s*\]$`)

// rewriteSCIMMemberFilterRemoval turns Okta's remove of members[value eq "id"]
// into a remove of members with that value, which the library accepts.
func rewriteSCIMMemberFilterRemoval(body []byte) []byte {
	var req map[string]json.RawMessage
	if json.Unmarshal(body, &req) != nil {
		return body
	}
	var ops []map[string]json.RawMessage
	if json.Unmarshal(req["Operations"], &ops) != nil {
		return body
	}
	changed := false
	for _, op := range ops {
		var path, verb string
		_ = json.Unmarshal(op["path"], &path)
		_ = json.Unmarshal(op["op"], &verb)
		m := scimMemberFilterPath.FindStringSubmatch(path)
		if m == nil || !strings.EqualFold(verb, "remove") {
			continue
		}
		val, err := json.Marshal([]map[string]string{{"value": m[1]}})
		if err != nil {
			return body
		}
		op["path"], op["value"], changed = json.RawMessage(`"members"`), val, true
	}
	if !changed {
		return body
	}
	rawOps, err := json.Marshal(ops)
	if err != nil {
		return body
	}
	req["Operations"] = rawOps
	out, err := json.Marshal(req)
	if err != nil {
		return body
	}
	return out
}

// scimUserPatchKeys are the User attributes the library applies; any other
// attribute in a PATCH makes it reject the whole request.
var scimUserPatchKeys = map[string]bool{
	"active": true, "username": true, "externalid": true, "displayname": true, "name": true, "emails": true,
}

func scimUserPatchSupported(attr string) bool {
	a := strings.ToLower(strings.TrimSpace(attr))
	return a == "" || scimUserPatchKeys[a] || strings.HasPrefix(a, "name.")
}

// dropUnsupportedSCIMUserPatch removes PATCH operations (and keys inside a
// pathless value) on attributes the directory does not store, such as title,
// phone numbers, enterprise extension fields and filtered emails paths. Entra
// ID and Okta send those in the same request as the attributes that matter,
// and rejecting the request would stall the whole sync for that user.
func dropUnsupportedSCIMUserPatch(body []byte) ([]byte, []string) {
	var req map[string]json.RawMessage
	if json.Unmarshal(body, &req) != nil {
		return body, nil
	}
	var ops []map[string]json.RawMessage
	if json.Unmarshal(req["Operations"], &ops) != nil {
		return body, nil
	}
	var dropped []string
	kept := make([]map[string]json.RawMessage, 0, len(ops))
	for _, op := range ops {
		var path string
		_ = json.Unmarshal(op["path"], &path)
		if path != "" {
			if !scimUserPatchSupported(path) {
				dropped = append(dropped, path)
				continue
			}
			kept = append(kept, op)
			continue
		}
		var obj map[string]json.RawMessage
		if json.Unmarshal(op["value"], &obj) != nil {
			kept = append(kept, op)
			continue
		}
		for k := range obj {
			if !scimUserPatchSupported(k) {
				dropped = append(dropped, k)
				delete(obj, k)
			}
		}
		if len(obj) == 0 {
			continue
		}
		raw, err := json.Marshal(obj)
		if err != nil {
			return body, nil
		}
		op["value"] = raw
		kept = append(kept, op)
	}
	if len(dropped) == 0 {
		return body, nil
	}
	rawOps, err := json.Marshal(kept)
	if err != nil {
		return body, nil
	}
	req["Operations"] = rawOps
	out, err := json.Marshal(req)
	if err != nil {
		return body, nil
	}
	return out, dropped
}

func scimActiveTrue(raw json.RawMessage) bool {
	var b bool
	if json.Unmarshal(raw, &b) == nil {
		return b
	}
	var s string
	return json.Unmarshal(raw, &s) == nil && strings.EqualFold(s, "true")
}

// scimPatchActivates reports whether a SCIM PatchOp sets active to true.
func scimPatchActivates(body []byte) bool {
	var req struct {
		Operations []struct {
			Path  string          `json:"path"`
			Value json.RawMessage `json:"value"`
		} `json:"Operations"`
	}
	if json.Unmarshal(body, &req) != nil {
		return false
	}
	for _, op := range req.Operations {
		if strings.EqualFold(op.Path, "active") && scimActiveTrue(op.Value) {
			return true
		}
		var obj struct {
			Active json.RawMessage `json:"active"`
		}
		if op.Path == "" && json.Unmarshal(op.Value, &obj) == nil && len(obj.Active) > 0 && scimActiveTrue(obj.Active) {
			return true
		}
	}
	return false
}

// scimMarkInactive makes a PATCH answer show the state the platform applied
// after the library's own answer, which still reports the user as active.
func scimMarkInactive(body []byte) []byte {
	var res map[string]json.RawMessage
	if json.Unmarshal(body, &res) != nil {
		return body
	}
	if _, ok := res["active"]; !ok {
		return body
	}
	res["active"] = json.RawMessage("false")
	out, err := json.Marshal(res)
	if err != nil {
		return body
	}
	return out
}

// scimLocation returns the meta.location of a created resource.
func scimLocation(body []byte) string {
	var res struct {
		Meta struct {
			Location string `json:"location"`
		} `json:"meta"`
	}
	if json.Unmarshal(body, &res) != nil {
		return ""
	}
	return res.Meta.Location
}

func scimInvalidIDAnswer(status int, body []byte) bool {
	if status != http.StatusBadRequest {
		return false
	}
	var res struct {
		Detail string `json:"detail"`
	}
	return json.Unmarshal(body, &res) == nil && res.Detail == "invalid id"
}
