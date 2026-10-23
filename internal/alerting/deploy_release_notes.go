package alerting

import (
	"os"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	// EnvReleaseNotesMaxBytes caps release notes attached to a deploy notification.
	EnvReleaseNotesMaxBytes     = "RELEASE_NOTES_MAX_BYTES"
	defaultReleaseNotesMaxBytes = 4000
	// EnvReleaseNotesPreviewBytes caps the excerpt sent to short-text channels.
	EnvReleaseNotesPreviewBytes     = "RELEASE_NOTES_PREVIEW_BYTES"
	defaultReleaseNotesPreviewBytes = 300
	releaseNotesTruncMarker         = "\n\n...(truncated)"
	releaseNotesPreviewLines        = 3
)

func envPositiveInt(key string, def int) int {
	if n, err := strconv.Atoi(strings.TrimSpace(os.Getenv(key))); err == nil && n > 0 {
		return n
	}
	return def
}

// ReleaseNotesMaxBytes reads the notes size cap from the environment.
func ReleaseNotesMaxBytes() int {
	return envPositiveInt(EnvReleaseNotesMaxBytes, defaultReleaseNotesMaxBytes)
}

// ClampReleaseNotes trims s to at most limit bytes (marker included) on a rune boundary.
func ClampReleaseNotes(s string, limit int) string {
	s = strings.TrimSpace(s)
	if limit <= 0 || len(s) <= limit {
		return s
	}
	keep := limit - len(releaseNotesTruncMarker)
	if keep < 0 {
		keep = 0
	}
	for keep > 0 && !utf8.RuneStart(s[keep]) {
		keep--
	}
	return strings.TrimRight(s[:keep], "\n ") + releaseNotesTruncMarker
}

// releaseNotesLong reports channels that take the full notes. Alerting-style
// kinds (paging, push) and unlisted kinds get none.
func releaseNotesLong(k NotifyKind) bool {
	switch k {
	case NotifySlack, NotifyMattermost, NotifyRocketChat, NotifyWebex,
		NotifyGoogleChat, NotifyTeams, NotifyEmail, NotifyResend:
		return true
	}
	return false
}

// releaseNotesPreview reports short-text chat kinds that get a few lines only.
func releaseNotesPreview(k NotifyKind) bool {
	switch k {
	case NotifyDiscord, NotifyTelegram, NotifyNtfy, NotifyGotify, NotifyLark:
		return true
	}
	return false
}

// deployText is summaryDeployText plus release notes sized for the channel.
func deployText(k NotifyKind, ev DeployOutcome) string {
	msg := summaryDeployText(ev)
	if !ev.Succeeded || ev.ReleaseNotes == "" {
		return msg
	}
	switch {
	case releaseNotesLong(k):
		return msg + "\n\n" + ev.ReleaseNotes
	case releaseNotesPreview(k):
		lines := strings.Split(ev.ReleaseNotes, "\n")
		if len(lines) > releaseNotesPreviewLines {
			lines = lines[:releaseNotesPreviewLines]
		}
		prev := ClampReleaseNotes(strings.Join(lines, "\n"), envPositiveInt(EnvReleaseNotesPreviewBytes, defaultReleaseNotesPreviewBytes))
		return msg + "\n\n" + prev
	}
	return msg
}
