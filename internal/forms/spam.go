package forms

import "strings"

// Spam reasons stored with a flagged submission.
const (
	SpamHoneypot = "honeypot"
	SpamLinks    = "too_many_links"
	SpamEmpty    = "empty"
	SpamManual   = "manual"
)

// Classify reports whether fields look like spam and returns them without the honeypot field.
// maxLinks <= 0 disables the link heuristic.
func Classify(fields map[string]string, honeypot string, maxLinks int) (clean map[string]string, spam bool, reason string) {
	clean = make(map[string]string, len(fields))
	for k, v := range fields {
		if honeypot != "" && k == honeypot {
			if strings.TrimSpace(v) != "" {
				spam, reason = true, SpamHoneypot
			}
			continue
		}
		clean[k] = v
	}
	if spam {
		return clean, true, reason
	}
	if len(clean) == 0 {
		return clean, true, SpamEmpty
	}
	if maxLinks > 0 {
		links := 0
		for _, v := range clean {
			lv := strings.ToLower(v)
			links += strings.Count(lv, "http://") + strings.Count(lv, "https://")
		}
		if links > maxLinks {
			return clean, true, SpamLinks
		}
	}
	return clean, false, ""
}
