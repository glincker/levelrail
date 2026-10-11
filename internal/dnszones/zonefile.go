package dnszones

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/miekg/dns"
)

// Export formats.
const (
	FormatJSON = "json"
	FormatBIND = "bind"
)

// Document is the JSON import and export shape.
type Document struct {
	Zone    string      `json:"zone"`
	Records []RecordSet `json:"records"`
}

// SortSets orders sets by name (apex first), type, then set identifier.
func SortSets(sets []RecordSet) {
	slices.SortFunc(sets, func(a, b RecordSet) int {
		if a.Name != b.Name {
			if a.Name == Apex {
				return -1
			}
			if b.Name == Apex {
				return 1
			}
			return strings.Compare(a.Name, b.Name)
		}
		if a.Type != b.Type {
			return strings.Compare(a.Type, b.Type)
		}
		return strings.Compare(a.SetIdentifier, b.SetIdentifier)
	})
}

// ExportJSON renders sets as an indented Document.
func ExportJSON(zone string, sets []RecordSet) ([]byte, error) {
	out := slices.Clone(sets)
	SortSets(out)
	b, err := json.MarshalIndent(Document{Zone: NormalizeDomain(zone), Records: out}, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("dnszones: export json: %w", err)
	}
	return b, nil
}

// ParseJSON reads a Document (or a bare array of sets).
func ParseJSON(data []byte) ([]RecordSet, error) {
	trimmed := strings.TrimSpace(string(data))
	if strings.HasPrefix(trimmed, "[") {
		var sets []RecordSet
		if err := json.Unmarshal(data, &sets); err != nil {
			return nil, fmt.Errorf("dnszones: parse json: %w", err)
		}
		return sets, nil
	}
	var doc Document
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("dnszones: parse json: %w", err)
	}
	return doc.Records, nil
}

// ExportBIND renders sets as a zone file. Routed and alias sets have no
// zone file form and are listed as comments so nothing is silently dropped.
func ExportBIND(zone string, sets []RecordSet) string {
	z := NormalizeDomain(zone)
	out := slices.Clone(sets)
	SortSets(out)
	var b strings.Builder
	fmt.Fprintf(&b, "$ORIGIN %s.\n", z)
	for _, rs := range out {
		if rs.Type == "SOA" {
			continue
		}
		if rs.Alias != nil || rs.SetIdentifier != "" {
			fmt.Fprintf(&b, "; skipped %s %s set %q: routing policies and aliases have no zone file form\n", rs.Name, rs.Type, rs.SetIdentifier)
			continue
		}
		for _, v := range rs.Values {
			fmt.Fprintf(&b, "%s\t%d\tIN\t%s\t%s\n", rs.Name, rs.TTL, rs.Type, bindValue(rs.Type, v))
		}
	}
	return b.String()
}

func bindValue(typ, v string) string {
	switch typ {
	case "TXT":
		return quoteTXT(v)
	case "CNAME", "NS":
		return v + "."
	case "MX":
		return absLastField(v, 1, "")
	case "SRV":
		return absLastField(v, 3, "")
	}
	return v
}

// ParseBIND parses zone file text into record sets relative to zone.
// SOA, apex NS and unsupported types are skipped with a warning.
func ParseBIND(text, zone string, defaultTTL int) ([]RecordSet, []string, error) {
	z := NormalizeDomain(zone)
	zp := dns.NewZoneParser(strings.NewReader(text), z+".", "")
	if defaultTTL > 0 {
		zp.SetDefaultTTL(uint32(defaultTTL)) //nolint:gosec // bounded by Normalize's TTLMax
	}
	var (
		sets     []RecordSet
		warnings []string
		index    = map[Key]int{}
	)
	for rr, ok := zp.Next(); ok; rr, ok = zp.Next() {
		h := rr.Header()
		typ := dns.TypeToString[h.Rrtype]
		name := RelativeName(h.Name, z)
		if fq := NormalizeDomain(h.Name); fq != z && !strings.HasSuffix(fq, "."+z) {
			warnings = append(warnings, fmt.Sprintf("%s is outside %s, skipped", fq, z))
			continue
		}
		if typ == "SOA" || (typ == "NS" && name == Apex) || !supportedType(typ) {
			warnings = append(warnings, fmt.Sprintf("%s %s skipped (provider managed or unsupported)", name, typ))
			continue
		}
		val := strings.TrimSpace(strings.TrimPrefix(rr.String(), h.String()))
		if typ == "TXT" {
			val, _ = FormatRR(rr)
		}
		k := Key{Name: name, Type: typ}
		if i, seen := index[k]; seen {
			sets[i].Values = append(sets[i].Values, val)
			continue
		}
		index[k] = len(sets)
		sets = append(sets, RecordSet{Name: name, Type: typ, TTL: int(h.Ttl), Values: []string{val}})
	}
	if err := zp.Err(); err != nil {
		return nil, warnings, fmt.Errorf("dnszones: parse zone file: %w", err)
	}
	return sets, warnings, nil
}
