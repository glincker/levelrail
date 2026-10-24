---
title: "Deploy to Levelrail button"
description: Add a Deploy to Levelrail button to your project's README so readers can launch it on their own server, with every URL parameter, limits and copy-paste snippets
"og:url": "https://docs.levelrail.com/deploy-button"
---

A "Deploy to Levelrail" button is a link you put in your README. A reader who clicks it lands on a page that asks for the address of their own Levelrail server, then opens the new-app form on that server with your image, name, port and health check already filled in. Nothing is created until they review the form and press Deploy.

## How it works

1. The button links to `https://levelrail.com/deploy` with query parameters.
2. That page asks the reader for their server address and remembers it in their browser only. Nothing is sent to any other server.
3. It forwards the parameters to `https://<their-server>/apps/new?deploy=1&...`.
4. The reader's own dashboard validates every parameter, drops anything invalid (and says which ones), and shows a form to review.

## URL format

```text
https://levelrail.com/deploy?image=<image>&name=<name>&port=<port>&env=<KEY1,KEY2>&health=<path>&source=<url>
```

Percent-encode values (`:` as `%3A`, `/` as `%2F`). Only `image` is required for the prefilled form; without a valid `image` the reader sees the normal new-app wizard with a notice.

| Parameter | Meaning | Limits |
| --- | --- | --- |
| `image` | Container image reference, such as `ghcr.io/you/app:1.2` | 255 characters. Lower case path, optional registry host and port, optional tag or `@sha256:` digest. Anything else is rejected |
| `name` | App name | Lower case letters, digits and hyphens, 1 to 63 characters, starting and ending with a letter or digit. Defaults to a name derived from the image |
| `port` | Port the app listens on inside its container | Integer 1 to 65535 |
| `env` | Names of environment variables the app needs, comma separated | Names only, `[A-Z_][A-Z0-9_]*`, up to 64 characters each, at most 50 names. `NAME=value` is rejected |
| `health` | HTTP health check path | Starts with a single `/`, up to 256 characters, no spaces, control characters or backslashes |
| `source` | Project page shown to the reader as "filled in by a link from <host>" | `http` or `https` URL with no username or password, up to 2048 characters. Only the host is displayed; it is not used to build anything |

Unknown parameters are ignored and listed in a notice on the reader's form. The public page forwards only the parameters above, each cut to 2048 characters.

## Badge

Two SVG badges live in this repository under `docs/assets/brand`: `deploy-button.svg` (yellow `#eef35f` with dark text) and `deploy-button-dark.svg` (dark with light text).

The snippets load the badge from the jsDelivr CDN, which serves the file straight from this public repository, so README views never hit the Levelrail site. The `@main` part follows the main branch; pin a release tag instead (for example `@v0.8.4`) if you want a badge that never changes. Only clicking the button reaches `levelrail.com/deploy`.

## Markdown snippet

```md
[![Deploy to Levelrail](https://cdn.jsdelivr.net/gh/glincker/levelrail@main/docs/assets/brand/deploy-button.svg)](https://levelrail.com/deploy?image=ghcr.io%2Fyou%2Fapp%3A1.2&port=3000&health=%2Fhealthz&env=DATABASE_URL)
```

## HTML snippet

```html
<a href="https://levelrail.com/deploy?image=ghcr.io%2Fyou%2Fapp%3A1.2&amp;port=3000&amp;health=%2Fhealthz&amp;env=DATABASE_URL">
  <img src="https://cdn.jsdelivr.net/gh/glincker/levelrail@main/docs/assets/brand/deploy-button.svg" alt="Deploy to Levelrail" height="40">
</a>
```

## Examples

An app published as a Docker image:

```text
https://levelrail.com/deploy?image=ghcr.io%2Fyou%2Fnotes%3A2.0&name=notes&port=8080&health=%2Fhealthz&env=DATABASE_URL,SESSION_KEY
```

An app that lives in a Git repository. `source` points readers at the project; the reader chooses the repository in the new-app wizard when the link has no image:

```text
https://levelrail.com/deploy?name=notes&port=3000&source=https%3A%2F%2Fgithub.com%2Fyou%2Fnotes
```

## Build the snippet in the dashboard

On your own server, open **Apps, New app** and use the "Add a Deploy to Levelrail button to your README" section below the wizard. It builds the link from an image, name, port, health path and env variable names, applies the same validation as above, and copies the Markdown.

## Security

- The button never carries secrets. Environment variable values are not read from a link, even if someone puts one there; only names are accepted.
- Secrets are entered by the reader on their own server, in password fields, and stored as secrets there.
- The public page does not contact any Levelrail server. It only redirects the browser to the address the reader typed.
- Nothing is deployed from a link alone. The reader reviews the form and must press Deploy.
- Only deploy links you trust: a link can pick any public image, so check the image and source host shown on the form.

See also [Templates and registry](/templates-and-registry) and [Deploying apps](/deploying-apps).
