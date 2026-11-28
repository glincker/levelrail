package envrules

import "sort"

// Rejection is one key the writer refused, with the reason. Values never appear.
type Rejection struct {
	Key    string `json:"key"`
	Code   Code   `json:"code"`
	Reason string `json:"reason"`
}

// Report is the per-key outcome of a bulk write such as a dotenv import.
type Report struct {
	Accepted []string    `json:"accepted"`
	Rejected []Rejection `json:"rejected"`
	Warnings []Issue     `json:"warnings"`
}

// BuildReport splits the keys of set into accepted and rejected using a Validate result.
func BuildReport(set map[string]string, res Result) Report {
	rep := Report{Accepted: []string{}, Rejected: []Rejection{}, Warnings: []Issue{}}
	refused := map[string]bool{}
	for _, e := range res.Errors {
		refused[e.Key] = true
		rep.Rejected = append(rep.Rejected, Rejection{Key: e.Key, Code: e.Code, Reason: e.Message})
	}
	for k := range set {
		if !refused[k] {
			rep.Accepted = append(rep.Accepted, k)
		}
	}
	sort.Strings(rep.Accepted)
	rep.Warnings = append(rep.Warnings, res.Warnings...)
	return rep
}
