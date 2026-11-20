package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

var pagesURLNameStrip = regexp.MustCompile(`(?i)\.(zip|tar\.gz|tgz)$`)

// pagesURLSiteName derives a site name from the archive file name.
func pagesURLSiteName(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	base := pagesURLNameStrip.ReplaceAllString(path.Base(u.Path), "")
	return strings.Trim(dbNamePattern.ReplaceAllString(strings.ToLower(base), "-"), "-")
}

// redactPagesURL drops the query string and userinfo so a signed URL is never printed.
func redactPagesURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "(invalid url)"
	}
	u.User, u.RawQuery, u.Fragment = nil, "", ""
	return u.String()
}

// runPagesDeployURL publishes an https .zip or .tar.gz the control plane downloads itself.
func runPagesDeployURL(prog, src string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if !strings.HasPrefix(strings.ToLower(src), "https://") {
		_, _ = fmt.Fprintf(stderr, "%s: --url must be an https URL\n", prog)
		return exitUsage
	}
	domain, rest, _ := pagesArgs(args)
	name := pagesFlagValue(rest, "app", "name")
	if name == "" {
		name = pagesURLSiteName(src)
	}
	if name == "" {
		_, _ = fmt.Fprintf(stderr, "%s: cannot derive a site name from the URL, pass --name\n", prog)
		return exitUsage
	}
	client := apiClientFromFlags(prog, pagesFlagValue(rest, "api-url"), pagesFlagValue(rest, "token"), pagesFlagValue(rest, "profile"), lookupEnv)
	shown := redactPagesURL(src)
	ctx := context.Background()
	res, err := client.CreateStaticSiteFromURL(ctx, name, src)
	var apiErr *apiclient.APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusConflict {
		// Only an existing static site is redeployed; any other app must not be overwritten.
		if _, gerr := client.GetStaticSite(ctx, name); gerr != nil {
			_, _ = fmt.Fprintf(stderr, "%s: an app named %q already exists and is not a static site, pass --name\n", prog, name)
			return exitUsage
		}
		var b apiclient.BuildTriggerResponse
		if b, err = client.DeployStaticSiteFromURL(ctx, name, src); err == nil {
			res = apiclient.StaticSiteCreated{DeployID: b.ID}
			res.Name = name
		} else if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusBadRequest && strings.Contains(apiErr.Message, "static site") {
			_, _ = fmt.Fprintf(stderr, "%s: an app named %q already exists and is not a static site, pass --name\n", prog, name)
			return exitUsage
		}
	}
	if err != nil {
		return reportError(stdout, stderr, false, fmt.Errorf("deploy site %q from %s: %w", name, shown, err))
	}
	_, _ = fmt.Fprintf(stdout, "site %s publishing from %s", name, shown)
	if res.DeployID != "" {
		_, _ = fmt.Fprintf(stdout, " (deploy %s)", res.DeployID)
	}
	_, _ = fmt.Fprintln(stdout)
	if domain == "" {
		return exitOK
	}
	d, err := client.AddStaticSiteDomain(ctx, name, domain)
	if err != nil {
		return reportError(stdout, stderr, false, fmt.Errorf("site %q is publishing but adding domain %q failed: %w", name, domain, err))
	}
	printPagesDomain(stderr, d)
	return exitOK
}
