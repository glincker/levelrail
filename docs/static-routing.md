---
title: "Static site routing rules"
sidebarTitle: "Static routing"
description: Redirects, rewrites, response headers, single page app fallback, a custom 404 page and clean URLs for a static site, kept in your repository and published with each deploy.
"og:url": "https://docs.levelrail.com/static-routing"
---

## What this covers

A static site (`build.type: static`, see [Static pages](/pages)) can carry its own routing rules in the repository. They are read at deploy time, compiled, and published together with the files, so a rollback restores the rules that went with that version of the site. This page covers the `_redirects` and `_headers` files, the matching `app.yaml` fields, the serving options, the order rules are evaluated in, and how to test them.

## Where rules come from

| Source | What it holds |
| --- | --- |
| `_redirects` in the published directory | Redirects, rewrites and status rules, in Netlify syntax |
| `_headers` in the published directory | Response headers per path, in Netlify syntax |
| `routes:` in `app.yaml` | The same rules as YAML |
| `headers:` in `app.yaml` | The same header rules as YAML |
| `static:` in `app.yaml` | Single page app fallback, clean URLs, trailing slash, 404 page |

The two files are removed from the served tree when the deploy is published, so visitors cannot download them. Rules from `app.yaml` rank before rules from `_redirects`.

## `_redirects`

One rule per line: `from [query matches] to [status[!]]`. Lines starting with `#` are comments.

```text
# Redirect a single page
/home            /                       301
# Splat: everything under /blog moves
/blog/*          /news/:splat
# Placeholders
/news/:year/:slug  /articles/:year/:slug
# Query string match
/store id=:id    /products/:id           301
# Rewrite (the URL does not change)
/app/*           /app/index.html         200
# Serve a page with a 404 status
/ecommerce/*     /store-closed.html      404
# Force a rule even when a file exists at the path
/index.html      /                       301!
```

- The default status is `301`. Supported statuses: `200` (rewrite), `301`, `302`, `303`, `307`, `308`, and `404`, `410`, `451` (answer with the page named by the destination).
- `:name` matches one path segment and `*` matches the rest of the path (last segment only). Both can be used in the destination. `*` is available as `:splat`.
- Matching ignores case and a trailing slash.
- The visitor's query string is passed on to a redirect destination that has no query of its own.
- A `200` rewrite must point at a path on this site. Rewriting to another site (a proxy) is not supported; use a path forwarder on the domain instead.
- Conditions such as `Country=`, `Language=`, `Role=` and `Cookie=` need an edge runtime and are not supported. A line that uses one is skipped and reported.

## `_headers`

A path line, then indented header lines. Use `! Name` to remove a header.

```text
/*
  X-Frame-Options: DENY
  X-Content-Type-Options: nosniff
/assets/*
  Cache-Control: public, max-age=31536000, immutable
  ! Server
```

Every block whose path matches the request applies. If the same header is set more than once, the values are joined with `, `. Headers the ingress manages (`Host`, `Content-Length`, `Connection`, `Transfer-Encoding`, `X-Forwarded-*` and a few others) cannot be set. Header names must be valid tokens and values must be a single line.

## `app.yaml`

```yaml
services:
  site:
    build: { type: static, command: npm run build, publish: dist }
    domains: [www.example.com]
    routes:
      - { from: /blog/*, to: "/news/:splat", status: 301 }
      - { from: /store, to: "/products/:id", query: { id: ":id" } }
      - { from: /index.html, to: /, status: 301, force: true }
    headers:
      - path: "/*"
        values: { X-Frame-Options: DENY }
        remove: [Server]
    static:
      spa: true
      cleanUrls: true
      trailingSlash: remove
      notFound: 404.html
```

| Field | Meaning |
| --- | --- |
| `routes[].from`, `to`, `status`, `force`, `query` | One rule, same meaning as a `_redirects` line. `query` maps a parameter name to a literal or `:name`. |
| `headers[].path`, `values`, `remove` | One header block. |
| `static.spa` | Serve `index.html` with status 200 for unknown paths that have no file extension. |
| `static.cleanUrls` | `/about` serves `about.html` or `about/index.html`, and `/about.html` redirects to `/about`. |
| `static.trailingSlash` | `add` or `remove` redirects to the chosen form; `off` (default) leaves paths as requested. |
| `static.notFound` | The page served with status 404. Defaults to `404.html` when it exists. |

These fields are only valid with `build.type: static`. A mistake in them fails the deploy before anything is published.

## Order of evaluation

1. **Rules**, in order: `app.yaml` `routes`, then `_redirects`. The first rule that matches wins. A rule that is not forced (`!` or `force: true`) is skipped when a file exists at the requested path, so a stray rule cannot hide real content.
2. **Files**: the exact file, then `path.html` when clean URLs are on, then `path/index.html`. Trailing slash and clean URL redirects happen here.
3. **Single page app fallback**, when `static.spa` is on: a path with no file and no extension serves `index.html` with status 200. A missing `/app.js` still returns 404.
4. **404 page**, served with status 404. If there is none, a plain 404 response.

A redirect that would send a request to the path it came from is skipped rather than looping.

## Safety

- Redirect destinations are either a path on this site or an absolute `http` or `https` URL. A destination that starts with `//`, contains backslashes or whitespace, carries credentials, or uses a placeholder in its host is rejected, so a rule cannot become an open redirect.
- Values captured from the request are re-escaped before they are placed in a `Location` header.
- Header names and values are checked for control characters, so a rule cannot inject a second header.
- Matching uses linear time regular expressions, so a crafted URL cannot stall the ingress.

## Limits

| Environment variable | Default | Meaning |
| --- | --- | --- |
| `APP_STATIC_ROUTES_MAX_RULES` | `1000` | Redirect and rewrite rules per site |
| `APP_STATIC_ROUTES_MAX_HEADER_BLOCKS` | `100` | Header blocks per site |
| `APP_STATIC_ROUTES_MAX_FILE_BYTES` | `262144` | Size of each of `_redirects` and `_headers` |
| `APP_STATIC_ROUTES_MAX_VALUE_LEN` | `2048` | Length of one path, destination or header value |

Going over a cap fails the deploy with the variable's name in the message.

## Problems and the deploy log

A bad line in `_redirects` or `_headers` is skipped, never applied half understood, and reported in the deploy log under **Compile routing rules** with its file and line number. The same problems are listed on the **Routing rules** page of the app. The previous deploy keeps serving until the new one is published, and a failed deploy leaves its rules untouched.

## See the rules and test a URL

The **Routing rules** page of a static app (Traffic section of the app menu) shows the parsed rules, header blocks, options and any problems. **Test a URL** runs a path through the live rules and files and shows which rule or file answered, the status, and each step considered. **Check rules before deploying** parses pasted `_redirects` or `_headers` text without deploying.

The same from the CLI and the API:

```bash
levelrail-cli static-sites routes show my-site
levelrail-cli static-sites routes test my-site /old --request-query "ref=x"
levelrail-cli static-sites routes lint my-site --redirects ./public/_redirects --headers ./public/_headers
```

| Method | Path | Ability |
| --- | --- | --- |
| `GET` | `/api/v1/apps/{name}/static-routes` | `read` |
| `POST` | `/api/v1/apps/{name}/static-routes/test` | `read` |
| `POST` | `/api/v1/apps/{name}/static-routes/lint` | `read` |

`lint` exits non-zero when the text has problems, so it fits a CI step.

## Rollback

Rules are stored beside the published files of each deploy. Rolling back to an earlier deploy (see [Rollback](/rollback)) serves that deploy's files and its rules together. Sites published before routing rules existed have none; deploy them once to enable the options.
