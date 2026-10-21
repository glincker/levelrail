package forms

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

// Parse failures; the API maps each to a distinct status.
var (
	ErrTooLarge        = errors.New("forms: body too large")
	ErrUnsupportedType = errors.New("forms: unsupported content type")
	ErrTooManyFields   = errors.New("forms: too many fields")
	ErrFieldTooLong    = errors.New("forms: field too long")
	ErrEmpty           = errors.New("forms: no fields")
	ErrMalformed       = errors.New("forms: malformed body")
)

// ParseRequest reads urlencoded, multipart or JSON bodies into string fields.
// Multipart files are never stored; only the file name is kept as the value.
func ParseRequest(r *http.Request, cfg Config) (map[string]string, error) {
	mt, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		return nil, ErrUnsupportedType
	}
	body := io.Reader(r.Body)
	if cfg.MaxBodyBytes > 0 {
		body = io.LimitReader(r.Body, cfg.MaxBodyBytes+1)
	}
	var fields map[string]string
	switch mt {
	case "application/x-www-form-urlencoded":
		fields, err = parseURLEncoded(body, cfg)
	case "multipart/form-data":
		fields, err = parseMultipart(body, params["boundary"], cfg)
	case "application/json":
		fields, err = parseJSON(body, cfg)
	default:
		return nil, ErrUnsupportedType
	}
	if err != nil {
		return nil, err
	}
	if len(fields) == 0 {
		return nil, ErrEmpty
	}
	return fields, checkLimits(fields, cfg)
}

func readAll(body io.Reader, cfg Config) ([]byte, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrMalformed, err)
	}
	if cfg.MaxBodyBytes > 0 && int64(len(data)) > cfg.MaxBodyBytes {
		return nil, ErrTooLarge
	}
	return data, nil
}

func parseURLEncoded(body io.Reader, cfg Config) (map[string]string, error) {
	data, err := readAll(body, cfg)
	if err != nil {
		return nil, err
	}
	values, err := url.ParseQuery(string(data))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrMalformed, err)
	}
	out := make(map[string]string, len(values))
	for k, v := range values {
		out[k] = strings.Join(v, ", ")
	}
	return out, nil
}

func parseMultipart(body io.Reader, boundary string, cfg Config) (map[string]string, error) {
	if boundary == "" {
		return nil, ErrMalformed
	}
	data, err := readAll(body, cfg)
	if err != nil {
		return nil, err
	}
	mr := multipart.NewReader(bytes.NewReader(data), boundary)
	out := map[string]string{}
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrMalformed, err)
		}
		name := part.FormName()
		if name == "" {
			continue
		}
		if fn := part.FileName(); fn != "" {
			out[name] = joinValue(out[name], "file: "+fn)
			continue
		}
		val, err := io.ReadAll(io.LimitReader(part, fieldReadLimit(cfg)))
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrMalformed, err)
		}
		out[name] = joinValue(out[name], string(val))
	}
}

func joinValue(prev, next string) string {
	if prev == "" {
		return next
	}
	return prev + ", " + next
}

func parseJSON(body io.Reader, cfg Config) (map[string]string, error) {
	data, err := readAll(body, cfg)
	if err != nil {
		return nil, err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrMalformed, err)
	}
	out := make(map[string]string, len(raw))
	for k, v := range raw {
		out[k] = jsonValueText(v)
	}
	return out, nil
}

// jsonValueText renders scalars as plain text, arrays of scalars comma-joined and objects as compact JSON.
func jsonValueText(v json.RawMessage) string {
	var s string
	if json.Unmarshal(v, &s) == nil {
		return s
	}
	var list []json.RawMessage
	if json.Unmarshal(v, &list) == nil {
		parts := make([]string, len(list))
		for i, item := range list {
			parts[i] = jsonValueText(item)
		}
		return strings.Join(parts, ", ")
	}
	if string(v) == "null" {
		return ""
	}
	return string(bytes.TrimSpace(v))
}

func checkLimits(fields map[string]string, cfg Config) error {
	if cfg.MaxFields > 0 && len(fields) > cfg.MaxFields {
		return ErrTooManyFields
	}
	if cfg.MaxFieldBytes > 0 {
		for k, v := range fields {
			if len(k) > 256 || len(v) > cfg.MaxFieldBytes {
				return ErrFieldTooLong
			}
		}
	}
	return nil
}

// SortedKeys returns the field names in a stable order for display and CSV columns.
func SortedKeys(fields map[string]string) []string {
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func fieldReadLimit(cfg Config) int64 {
	if cfg.MaxFieldBytes <= 0 {
		return cfg.MaxBodyBytes + 1
	}
	return int64(cfg.MaxFieldBytes) + 1
}
