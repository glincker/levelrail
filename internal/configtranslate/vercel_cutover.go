package configtranslate

// vercelCutoverChecklist is the ordered list appended to MIGRATION.md when the
// source is a Vercel project, so the DNS switch happens last.
func vercelCutoverChecklist() []string {
	return []string{
		"Cutover checklist (Vercel): deploy and test the app on its temporary URL before touching DNS.",
		"Cutover checklist: copy the environment variables (`vercel env pull`, then import the file as app secrets) and confirm the app boots.",
		"Cutover checklist: lower the DNS TTL on the domain a day ahead so the switch propagates fast.",
		"Cutover checklist: add the domain to the app, wait for the certificate, then point the DNS records at this server.",
		"Cutover checklist: keep the Vercel project until traffic is confirmed here, then remove the domain from it.",
	}
}

// hasFormat reports whether f was one of the detected source formats.
func (r Result) hasFormat(f Format) bool {
	for _, d := range r.Detected {
		if d == f {
			return true
		}
	}
	return false
}
