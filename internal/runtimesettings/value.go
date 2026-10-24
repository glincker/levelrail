package runtimesettings

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ValidationError is a rejected value, reported against the request's value field.
type ValidationError struct {
	Key     string
	Message string
}

func (e *ValidationError) Error() string { return e.Key + ": " + e.Message }

func parseBool(s string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	}
	return false, fmt.Errorf("%q is not a boolean", s)
}

func cleanList(items []string, check func(string) error) ([]string, error) {
	out := make([]string, 0, len(items))
	for _, it := range items {
		it = strings.TrimSpace(it)
		if it == "" {
			continue
		}
		if check != nil {
			if err := check(it); err != nil {
				return nil, err
			}
		}
		out = append(out, it)
	}
	return out, nil
}

// canonicalFromString validates s for d's kind and returns its canonical form:
// "true"/"false", a normalized duration, a decimal int, or a JSON array. A
// string_list accepts a JSON array or the colon separated env form.
func canonicalFromString(d Definition, s string) (string, error) {
	switch d.Kind {
	case KindBool:
		b, err := parseBool(s)
		if err != nil {
			return "", err
		}
		return strconv.FormatBool(b), nil
	case KindDuration:
		dur, err := ParseDuration(s)
		if err != nil {
			return "", err
		}
		return dur.String(), nil
	case KindInt:
		n, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil {
			return "", fmt.Errorf("%q is not an integer", s)
		}
		return strconv.Itoa(n), nil
	case KindStringList:
		var items []string
		if t := strings.TrimSpace(s); strings.HasPrefix(t, "[") {
			if err := json.Unmarshal([]byte(t), &items); err != nil {
				return "", fmt.Errorf("not a list of strings")
			}
		} else if t != "" {
			items = strings.Split(t, ":")
		}
		return canonicalList(d, items)
	}
	return "", fmt.Errorf("unknown kind %q", d.Kind)
}

func canonicalList(d Definition, items []string) (string, error) {
	clean, err := cleanList(items, d.CheckItem)
	if err != nil {
		return "", err
	}
	b, err := json.Marshal(clean)
	if err != nil {
		return "", fmt.Errorf("encode list: %w", err)
	}
	return string(b), nil
}

// ParseJSON validates an API value (bool, string, number or string array) and
// returns the canonical stored form.
func ParseJSON(d Definition, raw json.RawMessage) (string, error) {
	fail := func(msg string) (string, error) { return "", &ValidationError{Key: d.Key, Message: msg} }
	var v any
	if err := json.Unmarshal(raw, &v); err != nil || v == nil {
		return fail("value is required")
	}
	var s string
	switch d.Kind {
	case KindBool:
		switch x := v.(type) {
		case bool:
			s = strconv.FormatBool(x)
		case string:
			s = x
		default:
			return fail("value must be true or false")
		}
	case KindDuration:
		str, ok := v.(string)
		if !ok {
			return fail("value must be a duration string such as 10m")
		}
		s = str
	case KindInt:
		switch x := v.(type) {
		case float64:
			if x != float64(int64(x)) {
				return fail("value must be an integer")
			}
			s = strconv.FormatInt(int64(x), 10)
		case string:
			s = x
		default:
			return fail("value must be an integer")
		}
	case KindStringList:
		switch x := v.(type) {
		case string:
			s = x
		case []any:
			items := make([]string, 0, len(x))
			for _, e := range x {
				str, ok := e.(string)
				if !ok {
					return fail("every list entry must be a string")
				}
				items = append(items, str)
			}
			c, err := canonicalList(d, items)
			if err != nil {
				return fail(err.Error())
			}
			return c, nil
		default:
			return fail("value must be a list of strings")
		}
	}
	c, err := canonicalFromString(d, s)
	if err != nil {
		return fail(err.Error())
	}
	return c, nil
}

// typed renders a canonical value as its JSON type for the API.
func typed(d Definition, canonical string) json.RawMessage {
	switch d.Kind {
	case KindBool, KindInt, KindStringList:
		if canonical == "" {
			canonical = "[]"
		}
		return json.RawMessage(canonical)
	}
	b, _ := json.Marshal(canonical)
	return b
}

func listOf(canonical string) []string {
	var items []string
	_ = json.Unmarshal([]byte(canonical), &items)
	return items
}

func durationOf(canonical string) time.Duration {
	d, _ := time.ParseDuration(canonical)
	return d
}
