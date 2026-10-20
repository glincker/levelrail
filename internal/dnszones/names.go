package dnszones

import (
	"strconv"
	"strings"
)

// NormalizeDomain lowercases d and strips surrounding space and a trailing dot.
func NormalizeDomain(d string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(d)), ".")
}

// RelativeName turns an FQDN or relative name into a name relative to zone,
// with Apex for the zone itself.
func RelativeName(name, zone string) string {
	n := NormalizeDomain(name)
	z := NormalizeDomain(zone)
	switch {
	case n == "" || n == Apex || n == z:
		return Apex
	case strings.HasSuffix(n, "."+z):
		return strings.TrimSuffix(n, "."+z)
	}
	return n
}

// FQDN returns the absolute name (no trailing dot) of a relative name.
func FQDN(rel, zone string) string {
	z := NormalizeDomain(zone)
	r := RelativeName(rel, z)
	if r == Apex {
		return z
	}
	return r + "." + z
}

// unescapeOctal decodes Route53's \052 style escapes (it returns "*" as \052).
func unescapeOctal(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+4 <= len(s) {
			if v, err := strconv.ParseUint(s[i+1:i+4], 8, 8); err == nil {
				b.WriteByte(byte(v))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// splitTXT cuts text into the 255 byte character strings DNS requires.
func splitTXT(text string) []string {
	const limit = 255
	if text == "" {
		return []string{""}
	}
	var out []string
	for len(text) > limit {
		out = append(out, text[:limit])
		text = text[limit:]
	}
	return append(out, text)
}

// quoteTXT renders raw text as one or more quoted character strings.
func quoteTXT(text string) string {
	parts := splitTXT(text)
	for i, p := range parts {
		p = strings.ReplaceAll(p, `\`, `\\`)
		parts[i] = `"` + strings.ReplaceAll(p, `"`, `\"`) + `"`
	}
	return strings.Join(parts, " ")
}

// unquoteTXT joins quoted character strings back into raw text. Unquoted
// input is returned as is, since some providers hand back raw content.
func unquoteTXT(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, `"`) {
		return s
	}
	var b strings.Builder
	in := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\\' && in && i+1 < len(s):
			if i+3 < len(s) {
				if v, err := strconv.ParseUint(s[i+1:i+4], 10, 8); err == nil {
					b.WriteByte(byte(v))
					i += 3
					continue
				}
			}
			i++
			b.WriteByte(s[i])
		case c == '"':
			in = !in
		case in:
			b.WriteByte(c)
		}
	}
	return b.String()
}
