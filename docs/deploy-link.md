---
title: "Deploy links and badge"
description: "Put a Deploy with button in your README. A visitor enters their own server once and lands on a create-app form with your image, port and env names prefilled."
"og:url": "https://docs.levelrail.com/deploy-link"
---

A deploy link lets a project author add a button to a README. A visitor who clicks it picks their own server (remembered in their browser) and lands on that server's create-app form with the details filled in. Nothing is created until the visitor reviews the form and presses Deploy.

## URL scheme

```
https://<server>/apps/new?deploy=1&image=<ref>&name=<n>&port=<p>&env=KEY1,KEY2&health=/healthz&source=<url>
```

| Parameter | Meaning | Accepted values |
| --- | --- | --- |
| `deploy` | Marks the URL as a deploy link | `1` |
| `image` | Container image to run | An OCI reference such as `ghcr.io/you/app:1.2`, `nginx`, or `name@sha256:<64 hex>` |
| `name` | Suggested app name | A DNS label: lowercase letters, digits and hyphens, at most 63 characters. Derived from the image when omitted |
| `port` | Port the app listens on in its container | 1 to 65535 |
| `env` | Names of env variables the app needs | Comma separated, uppercase identifiers such as `DATABASE_URL`, at most 50 |
| `health` | Health check path | Starts with a single slash |
| `source` | Where the project lives, shown to the visitor | An http or https URL without a username or password |

Rules the form enforces:

- Env values are never accepted from a link, only the names. The visitor types each value, and it is stored as a secret.
- A parameter that fails validation is ignored, and a banner names every ignored parameter. Unknown parameters are ignored the same way.
- The form shows "Prefilled from a link: review before deploying" and names the source host when `source` is valid.
- If the link has no valid `image`, the regular new app wizard opens with the banner above it.

## Launcher page

Most READMEs should link to the launcher, because the author does not know the visitor's server:

```
https://levelrail.com/deploy.html?image=<ref>&name=<n>&port=<p>&env=KEY1,KEY2&health=/healthz&source=<url>
```

The launcher is a static page with a small script. It asks for the visitor's server address once, stores it in `localStorage` in that browser only, validates that it is an http or https address without a username or password, then redirects to `<server>/apps/new?deploy=1&...` carrying the same parameters. It makes no external requests and has no tracking. It does not add `deploy=1` to the link you write, the launcher does that.

## Badge

The badge is a flat SVG that reads on light and dark backgrounds:

```
https://levelrail.com/badges/deploy-with-levelrail.svg
```

Markdown for a README, using a real public image (`traefik/whoami`, port 80, health path `/health`):

```markdown
[![Deploy](https://levelrail.com/badges/deploy-with-levelrail.svg)](https://levelrail.com/deploy.html?image=traefik%2Fwhoami%3Alatest&name=whoami&port=80&health=%2Fhealth&source=https%3A%2F%2Fgithub.com%2Ftraefik%2Fwhoami)
```

Percent-encode `:` `/` and `,` inside parameter values as shown. For a project that needs secrets, add `env=API_KEY,DATABASE_URL`.

## Generate a link from the CLI

```
levelrail-cli deploy-link --image traefik/whoami:latest --name whoami --port 80 --health /health
```

It validates the same rules as the form and prints the launcher link and the README Markdown. Add `--server https://levelrail.example.com` to also print the direct link for one specific server, or `--launcher URL` to point at a self-hosted copy of the launcher. See the [CLI reference](/cli-reference#deploy-link).

## Self-hosting the launcher

`deploy.html` and `deploy.js` are plain static files under `docs/public`. Copy both, plus `badges/`, to any static host and pass that host to `--launcher`.
