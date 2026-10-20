// Command releasenotes renders a GitHub release body from the merged PRs
// between a tag and its predecessor, plus published assets and images,
// and splices it into the release's existing body between markers.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

type imageFlags []string

func (f *imageFlags) String() string     { return strings.Join(*f, ",") }
func (f *imageFlags) Set(v string) error { *f = append(*f, v); return nil }

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "releasenotes:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		images                                       imageFlags
		repo, tag, product, docsURL, installURL, out string
		apiBase                                      string
	)
	flag.StringVar(&repo, "repo", os.Getenv("GITHUB_REPOSITORY"), "owner/name")
	flag.StringVar(&tag, "tag", "", "release tag to render")
	flag.StringVar(&product, "product", "", "product name, used for the install env var prefix and binary names")
	flag.StringVar(&docsURL, "docs-url", "", "docs site base URL")
	flag.StringVar(&installURL, "install-url", "", "install script URL")
	flag.StringVar(&out, "out", "", "write the full new release body here (default stdout)")
	flag.StringVar(&apiBase, "api", "https://api.github.com", "GitHub API base URL")
	flag.Var(&images, "image", "container image name (repeatable)")
	flag.Parse()
	if repo == "" || tag == "" || product == "" {
		return errors.New("-repo, -tag, and -product are required")
	}
	if installURL == "" {
		installURL = fmt.Sprintf("https://raw.githubusercontent.com/%s/main/install.sh", repo)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	hc := &http.Client{Timeout: 30 * time.Second}
	token := os.Getenv("GH_TOKEN")
	if token == "" {
		token = os.Getenv("GITHUB_TOKEN")
	}
	gh := &Client{HTTP: hc, Base: apiBase, Token: token, Repo: repo}

	releases, err := gh.Releases(ctx)
	if err != nil {
		return fmt.Errorf("list releases: %w", err)
	}
	idx := -1
	for i, r := range releases {
		if r.TagName == tag {
			idx = i
		}
	}
	if idx < 0 {
		return fmt.Errorf("release %s not found in %s", tag, repo)
	}
	cur := releases[idx]
	rel := Release{
		Product: product, Repo: repo, Tag: tag, Prerelease: cur.Prerelease,
		DocsURL: docsURL, InstallURL: installURL,
	}
	for _, a := range cur.Assets {
		rel.Assets = append(rel.Assets, a.Name)
	}
	if idx > 0 {
		rel.PrevTag = releases[idx-1].TagName
	}
	if idx+1 < len(releases) {
		rel.NextTag = releases[idx+1].TagName
	}

	if rel.PrevTag != "" {
		if rel.Changes, err = gh.Changes(ctx, rel.PrevTag, tag); err != nil {
			return fmt.Errorf("collect changes: %w", err)
		}
		if rel.NewContributors, err = gh.NewContributors(ctx, rel.PrevTag, tag); err != nil {
			fmt.Fprintln(os.Stderr, "releasenotes: warning: new contributors unavailable:", err)
		}
	}
	rel.Contributors = Contributors(rel.Changes)

	for _, name := range images {
		d, err := Digest(ctx, hc, name, tag)
		if err != nil {
			return fmt.Errorf("resolve digest: %w", err)
		}
		if d != "" {
			rel.Images = append(rel.Images, Image{Name: name, Digest: d})
		}
	}

	body := Splice(cur.Body, Render(rel))
	if out == "" {
		_, err = os.Stdout.WriteString(body)
		return err
	}
	if err := os.WriteFile(out, []byte(body), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", out, err)
	}
	return nil
}
