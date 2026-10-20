package datamigrate

import (
	"bufio"
	"sort"
	"strconv"
	"strings"
)

// ParseCounts reads "name|count" lines, ignoring anything that is not one
// (server notices, blank lines). The last field is the count, so a table
// name containing "|" still parses.
func ParseCounts(out string) map[string]int64 {
	counts := map[string]int64{}
	sc := bufio.NewScanner(strings.NewReader(out))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		i := strings.LastIndex(line, "|")
		if i <= 0 {
			continue
		}
		n, err := strconv.ParseInt(strings.TrimSpace(line[i+1:]), 10, 64)
		if err != nil || n < 0 {
			continue
		}
		counts[strings.TrimSpace(line[:i])] = n
	}
	return counts
}

// Compare checks every source table exists on the target with the same count.
// Tables only the target has are ignored: restore tooling may add some.
func Compare(source, target map[string]int64) Verification {
	names := make([]string, 0, len(source))
	for n := range source {
		names = append(names, n)
	}
	sort.Strings(names)
	v := Verification{Checked: len(names)}
	for _, n := range names {
		t, ok := target[n]
		tc := TableCount{Name: n, Source: source[n], Target: t, OK: ok && t == source[n]}
		if !tc.OK {
			v.Mismatched++
		}
		v.Tables = append(v.Tables, tc)
	}
	return v
}

// Failure summarises a verification that did not pass, listing at most max tables.
func (v Verification) Failure(limit int) string {
	var parts []string
	for _, t := range v.Tables {
		if t.OK {
			continue
		}
		if len(parts) == limit {
			parts = append(parts, "...")
			break
		}
		parts = append(parts, t.Name+" source="+strconv.FormatInt(t.Source, 10)+" target="+strconv.FormatInt(t.Target, 10))
	}
	return strconv.Itoa(v.Mismatched) + " of " + strconv.Itoa(v.Checked) + " tables differ: " + strings.Join(parts, "; ")
}
