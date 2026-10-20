package api

import (
	"net/http"
	"strconv"
	"strings"
)

type deployAttemptFilter struct {
	Branch string
	Author string
	Commit string
	Status string
	Query  string
	Limit  int
}

func parseDeployAttemptFilter(r *http.Request) deployAttemptFilter {
	q := r.URL.Query()
	f := deployAttemptFilter{
		Branch: strings.TrimSpace(q.Get("branch")),
		Author: strings.ToLower(strings.TrimSpace(q.Get("author"))),
		Commit: strings.ToLower(strings.TrimSpace(q.Get("commit"))),
		Status: strings.TrimSpace(q.Get("status")),
		Query:  strings.ToLower(strings.TrimSpace(q.Get("q"))),
	}
	if n, err := strconv.Atoi(q.Get("limit")); err == nil && n > 0 {
		f.Limit = n
	}
	return f
}

func (f deployAttemptFilter) active() bool {
	return f.Branch != "" || f.Author != "" || f.Commit != "" || f.Status != "" || f.Query != "" || f.Limit > 0
}

func (f deployAttemptFilter) matches(a deployAttemptResource) bool {
	if f.Branch != "" && a.Branch != f.Branch {
		return false
	}
	if f.Status != "" && a.Status != f.Status {
		return false
	}
	if f.Author != "" && !strings.Contains(strings.ToLower(a.Author), f.Author) {
		return false
	}
	if f.Commit != "" && !strings.HasPrefix(strings.ToLower(a.CommitSHA), f.Commit) {
		return false
	}
	if f.Query != "" {
		hay := strings.ToLower(a.CommitMessage + " " + a.Image + " " + a.Branch + " " + a.Author + " " + a.CommitSHA)
		if !strings.Contains(hay, f.Query) {
			return false
		}
	}
	return true
}

func (f deployAttemptFilter) apply(in []deployAttemptResource) []deployAttemptResource {
	if !f.active() {
		return in
	}
	out := make([]deployAttemptResource, 0, len(in))
	for _, a := range in {
		if !f.matches(a) {
			continue
		}
		out = append(out, a)
		if f.Limit > 0 && len(out) >= f.Limit {
			break
		}
	}
	return out
}
