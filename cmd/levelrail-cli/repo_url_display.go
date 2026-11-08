package main

import (
	"fmt"

	"github.com/GLINCKER/levelrail/internal/platformimport"
)

// sanitizeRepoForReport cleans a repo URL for report output and records a
// review issue when a credential was removed.
func sanitizeRepoForReport(raw string, issues []migrationIssue) (string, []migrationIssue) {
	clean, changed := platformimport.DisplayRepoURL(raw)
	if changed {
		issues = appendIssue(issues, "repository", issueReview, fmt.Sprintf(
			"a credential was removed from the repository URL (%s); supply it with the git credential helper or a deploy key instead of inline",
			describeCleanRepo(clean)))
	}
	return clean, issues
}

func describeCleanRepo(clean string) string {
	if clean == "" {
		return "URL not shown"
	}
	return "now " + clean
}
