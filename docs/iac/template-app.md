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
metadata: { name: archive }
spec:
  template: archivebox
  name: archive           # optional, defaults to metadata.name
  env:                    # the template's install form
    ADMIN_USERNAME: admin
  domains:                # service: host, only for services the template offers
    archivebox: archive.example.com
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
metadata: { name: archive }
spec:
  template: archivebox
  env:
    ADMIN_USERNAME: admin
  domains:
    archivebox: archive.example.com
```

For a secret setting, write `${{ env.NAME }}` as the value and pass the variable on the CLI with `--allow-env NAME`. A literal that looks like a secret is refused.

```bash
levelrail apply -f archive.yaml
```

## What plan and apply check

- The template id must exist in the catalog of the control plane you apply to.
- Every `env` key must be a setting the template declares, and every required setting without a default must be present.
- Secret looking values must come from `${{ env.NAME }}`, never a literal in the file. The plan shows only a keyed hash of the env values.
- Apply uses the same install path as the dashboard, so the same permission (deploy) is needed.

## Install time only

`env` and `domains` are used once, at install. A later apply never changes an installed app, so editing them does not update it. When the control plane has a record of what was installed, a different `env` or `domains` fails the plan with `template_app_inputs_changed`, so CI can catch it. To change them, remove the app and apply again. Image tags stay pinned.

If an app with that name exists but its latest revision was not installed from the same template, the plan fails with an ownership conflict.

## Limits

- A stack never owns or prunes a `TemplateApp`, because installed apps hold data.
- A `TemplateApp` has no project, so it cannot be applied with `--project` or inside a project scoped stack. The plan says so.
- A server stack keeps its rendered files in every revision, so `env` there cannot use `${{ env.NAME }}`: the value would be stored and returned by the revisions API. Stack applies refuse it. Apply secrets with the CLI, outside a stack.
- Export does not emit `TemplateApp` yet, since the install form values are not recorded.
