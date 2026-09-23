package main

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	startMarker = "<!-- release-notes:start -->"
	endMarker   = "<!-- release-notes:end -->"
)

// Image is one published container image and its manifest-list digest.
type Image struct {
	Name   string
	Digest string
}

// Contributor is a first-time contributor and the PR that introduced them.
type Contributor struct {
	Login string
	PR    int
}

// Release is everything the renderer needs; it does no I/O itself.
type Release struct {
	Product         string
	Repo            string
	Tag             string
	PrevTag         string
	NextTag         string
	Prerelease      bool
	DocsURL         string
	InstallURL      string
	Changes         []Change
	Assets          []string
	Images          []Image
	Contributors    []string
	NewContributors []Contributor
}

// ImageTags returns the tags release.yml pushes for a release tag: the
// exact tag plus the moving channel tags.
func ImageTags(tag string, prerelease bool) []string {
	if prerelease || strings.Contains(tag, "-") {
		return []string{tag, "beta"}
	}
	tags := []string{tag}
	if m := regexp.MustCompile(`^(v\d+\.\d+)\.\d+$`).FindStringSubmatch(tag); m != nil {
		tags = append(tags, m[1])
	}
	return append(tags, "latest")
}

// Slug is the stable changelog page slug for a tag, e.g. v0-2-0-beta-7.
func Slug(tag string) string {
	return strings.NewReplacer(".", "-", "+", "-").Replace(strings.ToLower(tag))
}

func (r Release) prLink(n int) string {
	return fmt.Sprintf("[#%d](https://github.com/%s/pull/%d)", n, r.Repo, n)
}

func (r Release) line(c Change) string {
	var b strings.Builder
	b.WriteString("- ")
	if c.Scope != "" {
		fmt.Fprintf(&b, "**%s:** ", c.Scope)
	}
	b.WriteString(c.Text())
	if c.PR > 0 {
		fmt.Fprintf(&b, " (%s)", r.prLink(c.PR))
	} else if c.SHA != "" {
		fmt.Fprintf(&b, " ([%s](https://github.com/%s/commit/%s))", short(c.SHA), r.Repo, c.SHA)
	}
	if c.Author != "" && !strings.HasSuffix(c.Author, "[bot]") {
		fmt.Fprintf(&b, " by @%s", c.Author)
	}
	return b.String()
}

func short(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

func (r Release) hasBinaries() bool {
	for _, a := range r.Assets {
		if a != "checksums.txt" {
			return true
		}
	}
	return false
}

// Render produces the generated block, markers included.
func Render(r Release) string {
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format, a...) }
	w("%s\n", startMarker)

	if !r.hasBinaries() {
		w("> [!CAUTION]\n> This release shipped no binaries or container images (its build did not publish).")
		if r.NextTag != "" {
			w(" Use [%s](https://github.com/%s/releases/tag/%s) instead, which includes every change below.", r.NextTag, r.Repo, r.NextTag)
		}
		w("\n\n")
	} else if r.Prerelease {
		w("> [!NOTE]\n> This is a pre-release on the `beta` channel. Pin this exact version for anything you care about staying still.\n\n")
	}

	if hl := Highlights(r.Changes, 3); len(hl) > 0 && worthHighlighting(r.Changes, hl) {
		w("## Highlights\n\n")
		for _, c := range hl {
			w("%s\n", r.line(c))
		}
		w("\n")
	}

	if br := Breaking(r.Changes); len(br) > 0 {
		w("## Upgrade notes\n\n> [!WARNING]\n> This release contains breaking changes. Read these before upgrading.\n\n")
		for _, c := range br {
			note := c.BreakingNote
			if note == "" {
				note = c.Text()
			}
			w("- %s", note)
			if c.PR > 0 {
				w(" (%s)", r.prLink(c.PR))
			}
			w("\n")
		}
		w("\n")
	}

	sections, maintenance := Group(r.Changes)
	if len(sections) > 0 || len(maintenance) > 0 {
		w("## What's changed\n\n")
		for _, s := range sections {
			w("### %s\n\n", s.Title)
			for _, c := range s.Changes {
				w("%s\n", r.line(c))
			}
			w("\n")
		}
		if len(maintenance) > 0 {
			w("<details>\n<summary>Maintenance, CI, and dependency updates (%d)</summary>\n\n", len(maintenance))
			for _, c := range maintenance {
				w("%s\n", r.line(c))
			}
			w("\n</details>\n\n")
		}
	}

	if r.hasBinaries() {
		r.renderInstall(&b)
	}
	r.renderImages(&b)
	if r.hasBinaries() || len(r.Images) > 0 {
		r.renderVerify(&b)
	}
	r.renderContributors(&b)
	r.renderFooter(&b)

	w("%s\n", endMarker)
	return b.String()
}

// worthHighlighting skips the section when it would just repeat every
// feature listed under What's changed.
func worthHighlighting(changes, hl []Change) bool {
	if hl[0].HasLabel("highlight") {
		return true
	}
	feats := 0
	for _, c := range changes {
		if c.Type == "feat" && !c.NoteNone {
			feats++
		}
	}
	return feats > len(hl)
}

func (r Release) renderInstall(b *strings.Builder) {
	fmt.Fprintf(b, "## Install\n\nFresh install on a Linux host, pinned to this release:\n\n```sh\n")
	fmt.Fprintf(b, "curl -fsSL %s | sudo env %s_VERSION=%s sh\n```\n\n", r.InstallURL, strings.ToUpper(r.Product), r.Tag)
	fmt.Fprintf(b, "Upgrade an existing install in place (keeps the unit file and data):\n\n```sh\n")
	fmt.Fprintf(b, "curl -fsSL %s | sudo env %s_VERSION=%s sh -s upgrade\n```\n\n", r.InstallURL, strings.ToUpper(r.Product), r.Tag)
	if len(r.Images) > 0 {
		fmt.Fprintf(b, "Docker Compose: pin the image tag in `docker-compose.yml`:\n\n```yaml\nservices:\n  %s:\n    image: %s:%s\n```\n\n", strings.ToLower(r.Product), r.Images[0].Name, r.Tag)
	}
}

func (r Release) renderImages(b *strings.Builder) {
	if len(r.Images) == 0 {
		return
	}
	tags := ImageTags(r.Tag, r.Prerelease)
	moving := strings.Join(tags[1:], "`, `")
	fmt.Fprintf(b, "## Container images\n\nMulti-arch (`linux/amd64`, `linux/arm64`), signed with cosign, SBOM and provenance attached. Also tagged `%s` at release time (moving tags).\n\n", moving)
	fmt.Fprintf(b, "| Image | Tag | Digest |\n| --- | --- | --- |\n")
	for _, img := range r.Images {
		digest := "not published"
		if img.Digest != "" {
			digest = "`" + img.Digest + "`"
		}
		fmt.Fprintf(b, "| `%s` | `%s` | %s |\n", img.Name, r.Tag, digest)
	}
	b.WriteString("\n")
}

func (r Release) renderVerify(b *strings.Builder) {
	b.WriteString("## Verify\n\n")
	if r.hasBinaries() {
		fmt.Fprintf(b, "Binaries: check downloads against `checksums.txt`:\n\n```sh\ngh release download %s --repo %s --pattern '%s-linux-amd64' --pattern checksums.txt\nsha256sum --ignore-missing -c checksums.txt\n```\n\n", r.Tag, r.Repo, strings.ToLower(r.Product))
	}
	var img *Image
	for i := range r.Images {
		if r.Images[i].Digest != "" {
			img = &r.Images[i]
			break
		}
	}
	if img != nil {
		identity := fmt.Sprintf(`^https://github\.com/%s/\.github/workflows/release\.yml@refs/(heads/main|tags/v.+)$`, regexp.QuoteMeta(r.Repo))
		fmt.Fprintf(b, "Images: verify the keyless signature was made by this repository's release workflow:\n\n```sh\ncosign verify %s@%s \\\n  --certificate-identity-regexp '%s' \\\n  --certificate-oidc-issuer https://token.actions.githubusercontent.com\n```\n\n", img.Name, img.Digest, identity)
	}
}

func (r Release) renderContributors(b *strings.Builder) {
	if len(r.NewContributors) == 0 && len(r.Contributors) == 0 {
		return
	}
	b.WriteString("## Contributors\n\n")
	for _, c := range r.NewContributors {
		fmt.Fprintf(b, "- @%s made their first contribution in %s\n", c.Login, r.prLink(c.PR))
	}
	if len(r.NewContributors) > 0 {
		b.WriteString("\n")
	}
	if len(r.Contributors) > 0 {
		fmt.Fprintf(b, "Thanks to @%s.\n\n", strings.Join(r.Contributors, ", @"))
	}
}

func (r Release) renderFooter(b *strings.Builder) {
	var links []string
	if r.PrevTag != "" {
		links = append(links, fmt.Sprintf("**Full changelog:** [%s...%s](https://github.com/%s/compare/%s...%s)", r.PrevTag, r.Tag, r.Repo, r.PrevTag, r.Tag))
	}
	if r.DocsURL != "" {
		docs := strings.TrimSuffix(r.DocsURL, "/")
		links = append(links,
			fmt.Sprintf("[Release page](%s/changelog/%s)", docs, Slug(r.Tag)),
			fmt.Sprintf("[Installing](%s/installing)", docs),
			fmt.Sprintf("[Upgrading](%s/installing#upgrading)", docs),
			fmt.Sprintf("[Verifying signatures](%s/installing#verifying-image-signatures)", docs))
	}
	if len(links) > 0 {
		fmt.Fprintf(b, "%s\n\n", strings.Join(links, " | "))
	}
}

// Splice replaces the generated block in an existing release body, keeping
// anything written outside the markers. A body without markers is the
// release-please default and is replaced whole.
func Splice(existing, generated string) string {
	start := strings.Index(existing, startMarker)
	end := strings.Index(existing, endMarker)
	if start < 0 || end < start {
		return generated
	}
	end += len(endMarker)
	rest := strings.TrimPrefix(existing[end:], "\n")
	return existing[:start] + strings.TrimSuffix(generated, "\n") + "\n" + rest
}
