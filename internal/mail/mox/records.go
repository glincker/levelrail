package mox

import (
	"sort"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/mail"
)

// ParseRecords turns zone-file lines into records, skipping blanks and
// comments. A line that does not parse is skipped rather than failing the set.
func ParseRecords(lines []string) []mail.DNSRecord {
	var out []mail.DNSRecord
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, ";") {
			continue
		}
		f := strings.Fields(l)
		if len(f) < 3 {
			continue
		}
		rec := mail.DNSRecord{Name: strings.TrimSuffix(f[0], ".")}
		i := 1
		if n, err := strconv.Atoi(f[i]); err == nil {
			rec.TTL = n
			i++
		}
		if i < len(f) && strings.EqualFold(f[i], "IN") {
			i++
		}
		if i >= len(f) {
			continue
		}
		rec.Type = strings.ToUpper(f[i])
		rest := strings.TrimSpace(afterFields(l, i+1))
		if rec.Type == "TXT" {
			rec.Value = joinQuoted(rest)
		} else {
			rec.Value = rest
		}
		out = append(out, rec)
	}
	return out
}

// afterFields returns the text after the first n whitespace-separated fields.
func afterFields(s string, n int) string {
	for i := 0; i < n; i++ {
		s = strings.TrimLeft(s, " \t")
		j := strings.IndexAny(s, " \t")
		if j < 0 {
			return ""
		}
		s = s[j:]
	}
	return s
}

// joinQuoted concatenates the quoted chunks of a TXT value, dropping any
// trailing comment after the last quote.
func joinQuoted(s string) string {
	var b strings.Builder
	for {
		i := strings.IndexByte(s, '"')
		if i < 0 {
			break
		}
		s = s[i+1:]
		j := strings.IndexByte(s, '"')
		if j < 0 {
			b.WriteString(s)
			break
		}
		b.WriteString(s[:j])
		s = s[j+1:]
	}
	if b.Len() == 0 {
		return strings.TrimSpace(s)
	}
	return b.String()
}

// ExtractDKIM finds DKIM TXT records and returns the selector and bare public
// key of each, in selector order, so the DNS engine can build the record.
func ExtractDKIM(recs []mail.DNSRecord) []mail.DKIMKey {
	var out []mail.DKIMKey
	for _, r := range recs {
		if r.Type != "TXT" {
			continue
		}
		sel, _, ok := strings.Cut(r.Name, "._domainkey.")
		if !ok || sel == "" || !strings.Contains(r.Value, "v=DKIM1") {
			continue
		}
		key := tagValue(r.Value, "p")
		if key == "" {
			continue
		}
		out = append(out, mail.DKIMKey{Selector: sel, PublicKey: key, TXT: r.Value})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Selector < out[j].Selector })
	return out
}

func tagValue(txt, tag string) string {
	for _, part := range strings.Split(txt, ";") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if ok && strings.EqualFold(strings.TrimSpace(k), tag) {
			return strings.ReplaceAll(strings.TrimSpace(v), " ", "")
		}
	}
	return ""
}
