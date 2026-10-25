package main

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/deploylink"
)

// The static badge lives in this repository and is served from its default branch.
const (
	deployBadgePath = "/main/docs/assets/deploy-to-levelrail.svg"
	deployBadgeURL  = "https://raw.githubusercontent.com/glincker/levelrail" + deployBadgePath
)

const deployLinkOpenTimeout = 5 * time.Second

type deployRepoLinkResult struct {
	URL      string `json:"url"`
	Markdown string `json:"markdown"`
	HTML     string `json:"html"`
}

// runDeployLinkRepo prints a /deploy link for a git repository or template on
// the current profile's instance. The dashboard only prefills; nothing deploys.
func runDeployLinkRepo(prog string, p deploylink.Params, instance string, open bool, stdout, stderr io.Writer, of outputFlags) int {
	link, err := p.URL(instance)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", prog, err)
		return exitValidation
	}
	alt, badge := "Deploy", deployBadgeURL
	if b, berr := brand.LoadEmbedded(); berr == nil {
		alt = "Deploy to " + b.Name
		if slug := b.RepoSlug(); slug != "" {
			badge = "https://raw.githubusercontent.com/" + slug + deployBadgePath
		}
	}
	out := deployRepoLinkResult{
		URL:      link,
		Markdown: deploylink.Markdown(alt, link, badge),
		HTML:     deploylink.HTML(alt, link, badge),
	}
	if err := renderResult(stdout, of.Format, of.Query, out, func() {
		_, _ = fmt.Fprintf(stdout, "Link:\n%s\n\nREADME Markdown:\n%s\n\nREADME HTML:\n%s\n", out.URL, out.Markdown, out.HTML)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	if open {
		ctx, cancel := context.WithTimeout(context.Background(), deployLinkOpenTimeout)
		defer cancel()
		if err := openBrowserFunc(ctx, link); err != nil {
			_, _ = fmt.Fprintf(stderr, "%s: %v\n", prog, err)
			return exitNetwork
		}
	}
	return exitOK
}
