package api

import "github.com/GLINCKER/levelrail/internal/upgradehistory"

// cleanReleaseNotes strips HTML comments (such as the release-notes markers
// the release workflow wraps notes in) and surrounding blank space. Markdown
// and GitHub alert syntax are left for the client to render safely.
func cleanReleaseNotes(body string) string { return upgradehistory.CleanNotes(body) }
