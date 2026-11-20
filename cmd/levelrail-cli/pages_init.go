package main

import (
	"errors"
	"fmt"
	"html"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// pagesStarterBase mirrors the dashboard starter gallery so both front doors give the same sites.
const pagesStarterBase = `:root{color-scheme:light dark;--bg:oklch(1 0 0);--fg:oklch(0.2 0.02 265);--muted:oklch(0.5 0.03 265);--line:oklch(0.92 0.01 265);--accent:oklch(0.55 0.2 265);--accent-fg:oklch(1 0 0);--card:oklch(0.97 0.005 265)}
@media(prefers-color-scheme:dark){:root{--bg:oklch(0.18 0.015 265);--fg:oklch(0.95 0.01 265);--muted:oklch(0.72 0.03 265);--line:oklch(0.3 0.02 265);--accent:oklch(0.72 0.14 265);--accent-fg:oklch(0.18 0.015 265);--card:oklch(0.22 0.015 265)}}
*{box-sizing:border-box}
body{margin:0;font:16px/1.6 system-ui,-apple-system,Segoe UI,sans-serif;background:var(--bg);color:var(--fg)}
a{color:var(--accent)}
a:focus-visible,button:focus-visible{outline:3px solid var(--accent);outline-offset:2px}
.wrap{max-width:60rem;margin:0 auto;padding:0 1.25rem}
.btn{display:inline-block;padding:.65rem 1.1rem;border-radius:.5rem;background:var(--accent);color:var(--accent-fg);text-decoration:none;font-weight:600}
.muted{color:var(--muted)}
@media(prefers-reduced-motion:reduce){*{scroll-behavior:auto!important;transition:none!important}}`

type pagesStarter struct {
	css  string
	body func(n string) string
}

var pagesStarters = map[string]pagesStarter{
	"blank": {
		css: "main{padding:4rem 0}",
		body: func(n string) string {
			return `<main class="wrap"><h1>` + n + `</h1><p class="muted">Replace this page with your own.</p></main>`
		},
	},
	"landing": {
		css: `header,section,footer{padding:3rem 0}
.hero{padding:5rem 0 3rem}
h1{font-size:clamp(2rem,6vw,3.25rem);line-height:1.1;margin:0 0 1rem}
.grid{display:grid;gap:1rem;grid-template-columns:repeat(auto-fit,minmax(14rem,1fr))}
.card{background:var(--card);border:1px solid var(--line);border-radius:.75rem;padding:1.25rem}
.card h3{margin:0 0 .35rem}
footer{border-top:1px solid var(--line)}`,
		body: func(n string) string {
			return `<header class="wrap"><strong>` + n + `</strong></header>
<main>
<section class="wrap hero">
<h1>One clear line about what you make</h1>
<p class="muted">A short supporting sentence that says who it is for and why it helps.</p>
<p><a class="btn" href="#features">See what it does</a></p>
</section>
<section class="wrap" id="features" aria-labelledby="f">
<h2 id="f">Features</h2>
<div class="grid">
<div class="card"><h3>Fast</h3><p class="muted">Served as plain static files.</p></div>
<div class="card"><h3>Simple</h3><p class="muted">Edit the HTML and publish again.</p></div>
<div class="card"><h3>Yours</h3><p class="muted">Connect your own domain later.</p></div>
</div>
</section>
</main>
<footer class="wrap muted">&copy; ` + n + `</footer>`
		},
	},
	"docs": {
		css: `.layout{display:grid;gap:2rem;grid-template-columns:12rem 1fr;padding:2rem 0}
nav{position:sticky;top:1rem;align-self:start}
nav ul{list-style:none;margin:0;padding:0}
nav a{display:block;padding:.3rem 0;text-decoration:none}
pre{background:var(--card);border:1px solid var(--line);border-radius:.5rem;padding:1rem;overflow:auto}
h1{margin-top:0}
@media(max-width:40rem){.layout{grid-template-columns:1fr}nav{position:static}}`,
		body: func(n string) string {
			return `<div class="wrap layout">
<nav aria-label="Docs">
<strong>` + n + `</strong>
<ul>
<li><a href="#intro">Introduction</a></li>
<li><a href="#install">Install</a></li>
<li><a href="#usage">Usage</a></li>
</ul>
</nav>
<main>
<h1 id="intro">Introduction</h1>
<p class="muted">What this project is and who should read these docs.</p>
<h2 id="install">Install</h2>
<pre><code>npm install ` + n + `</code></pre>
<h2 id="usage">Usage</h2>
<p>Show the smallest working example first, then link to deeper guides.</p>
</main>
</div>`
		},
	},
	"portfolio": {
		css: `header{padding:4rem 0 2rem}
h1{font-size:clamp(2rem,6vw,3rem);margin:0}
.grid{display:grid;gap:1rem;grid-template-columns:repeat(auto-fill,minmax(15rem,1fr));padding:1rem 0 3rem}
.work{background:var(--card);border:1px solid var(--line);border-radius:.75rem;padding:1.25rem}
.work h3{margin:0 0 .35rem}
footer{border-top:1px solid var(--line);padding:2rem 0}`,
		body: func(n string) string {
			return `<header class="wrap">
<h1>` + n + `</h1>
<p class="muted">Designer and developer. A sentence about what you do and where.</p>
</header>
<main class="wrap">
<h2>Selected work</h2>
<div class="grid">
<article class="work"><h3>Project one</h3><p class="muted">What it was and what you did.</p></article>
<article class="work"><h3>Project two</h3><p class="muted">What it was and what you did.</p></article>
<article class="work"><h3>Project three</h3><p class="muted">What it was and what you did.</p></article>
</div>
</main>
<footer class="wrap"><a href="mailto:you@example.com">you@example.com</a></footer>`
		},
	},
}

func pagesStarterNames() []string {
	names := make([]string, 0, len(pagesStarters))
	for k := range pagesStarters {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

func pagesHTML(title, body, css string) string {
	return "<!doctype html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n<meta name=\"viewport\" content=\"width=device-width,initial-scale=1\">\n<meta name=\"color-scheme\" content=\"light dark\">\n<title>" + title + "</title>\n<style>" + pagesStarterBase + css + "</style>\n</head>\n<body>\n" + body + "\n</body>\n</html>\n"
}

// pagesInitFiles returns the files of a deployable site, keyed by relative path.
func pagesInitFiles(name, starter string) (map[string]string, error) {
	s, ok := pagesStarters[starter]
	if !ok {
		return nil, fmt.Errorf("unknown starter %q (choose %s)", starter, strings.Join(pagesStarterNames(), ", "))
	}
	n := html.EscapeString(name)
	title := n
	if starter == "docs" {
		title = n + " docs"
	}
	return map[string]string{
		"index.html": pagesHTML(title, s.body(n), s.css),
		"404.html":   pagesHTML("Page not found", `<main class="wrap"><h1>Page not found</h1><p class="muted">That page does not exist.</p><p><a class="btn" href="/">Back home</a></p></main>`, "main{padding:4rem 0}"),
		"_redirects": "# from to [status]\n# /old-page /new-page 301\n",
		"_headers":   "# Response headers per path\n/*\n  X-Content-Type-Options: nosniff\n  Referrer-Policy: strict-origin-when-cross-origin\n",
		"app.yaml":   "services:\n  " + name + ":\n    build: { type: static }\n",
	}, nil
}

// runPagesInit scaffolds a deployable static site and never overwrites a file.
func runPagesInit(prog string, args []string, stdout, stderr io.Writer) int {
	dir, starter := ".", "blank"
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-h" || a == "--help":
			_, _ = fmt.Fprintf(stdout, "Usage: %s pages init [dir] [--starter %s]\n\nWrites index.html, 404.html, _redirects, _headers and app.yaml. Existing files are never overwritten.\n", prog, strings.Join(pagesStarterNames(), "|"))
			return exitOK
		case a == "--starter" && i+1 < len(args):
			i++
			starter = args[i]
		case strings.HasPrefix(a, "--starter="):
			starter = strings.TrimPrefix(a, "--starter=")
		case strings.HasPrefix(a, "-"):
			_, _ = fmt.Fprintf(stderr, "%s: unknown flag %q for pages init\n", prog, a)
			return exitUsage
		default:
			dir = a
		}
	}
	name := dirAppName(dir)
	if name == "" {
		name = "my-site"
	}
	files, err := pagesInitFiles(name, starter)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", prog, err)
		return exitUsage
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: create %s: %v\n", prog, dir, err)
		return exitValidation
	}
	order := []string{"index.html", "404.html", "_redirects", "_headers", "app.yaml"}
	for _, rel := range order {
		path := filepath.Join(dir, rel)
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644) //nolint:gosec // site files are meant to be world-readable
		if errors.Is(err, os.ErrExist) {
			_, _ = fmt.Fprintf(stdout, "skipped %s (already exists)\n", path)
			continue
		}
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "%s: write %s: %v\n", prog, path, err)
			return exitValidation
		}
		_, werr := f.WriteString(files[rel])
		if cerr := f.Close(); werr == nil {
			werr = cerr
		}
		if werr != nil {
			_, _ = fmt.Fprintf(stderr, "%s: write %s: %v\n", prog, path, werr)
			return exitValidation
		}
		_, _ = fmt.Fprintf(stdout, "created %s\n", path)
	}
	_, _ = fmt.Fprintf(stdout, "next: %s pages deploy %s\n", prog, dir)
	return exitOK
}
