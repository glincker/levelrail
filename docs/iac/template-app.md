---
title: "Catalog templates as code"
tag: "Beta"
description: Install any catalog template by id from a stack file with a TemplateApp document, including the install form values and domains.
"og:url": "https://docs.levelrail.com/iac/template-app"
---

A `TemplateApp` document installs a catalog template the same way the dashboard install form does. You name the template, fill its form, and apply. There is no compose file to copy.

## The shape

```yaml
version: 1
kind: TemplateApp
metadata: { name: blog }
spec:
  template: ghost
  name: blog              # optional, defaults to metadata.name
  env:                    # the template's install form
    ADMIN_PASSWORD: ${{ env.GHOST_ADMIN_PASSWORD }}
  domains:                # service: host, only for services the template offers
    web: blog.example.com
```

| Field | Meaning |
| --- | --- |
| `template` | Required. A catalog template id. |
| `name` | App name, lowercase letters, digits and dashes, 40 characters at most. |
| `env` | Values for the template's install form. Keys must be settings the template declares. |
| `domains` | A host per service the template offers a domain for. |

## Example without a form

```yaml
version: 1
kind: TemplateApp
metadata: { name: status }
spec:
  template: uptime-kuma
```

## Example with an install form

```yaml
version: 1
kind: TemplateApp
metadata: { name: blog }
spec:
  template: ghost
  env:
    ADMIN_PASSWORD: ${{ env.GHOST_ADMIN_PASSWORD }}
  domains:
    web: blog.example.com
```

```bash
GHOST_ADMIN_PASSWORD=change-me levelrail apply -f blog.yaml --allow-env GHOST_ADMIN_PASSWORD
```

## What plan and apply check

- The template id must exist in the catalog of the control plane you apply to.
- Every `env` key must be a setting the template declares, and every required setting without a default must be present.
- Secret looking values must come from `${{ env.NAME }}`, never a literal in the file. The plan shows only a keyed hash of the env values.
- Apply uses the same install path as the dashboard, so the same permission (deploy) is needed.

## Re-applying

A second apply with the same file changes nothing. Once an app exists, its template image tags stay pinned, so a later apply does not change the env, domains or images of an installed app. To change those, use the dashboard or recreate the app. If an app with that name exists but was not installed from the same template, the plan fails with an ownership conflict.

A stack never owns or prunes a `TemplateApp`, because installed apps hold data. Export does not emit `TemplateApp` yet, since the install form values are not recorded.
