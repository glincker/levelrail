---
title: "Catalog index"
description: The versioned JSON file that describes every template in the Levelrail catalog, used by the docs, the marketing site and a future catalog API.
"og:url": "https://docs.levelrail.com/catalog-index"
---

The template catalog is published as one JSON file, `scripts/gen-template-gallery/catalog-index.json`. It is generated from the catalog by `go run ./scripts/gen-template-gallery`, and CI fails when the committed copy is stale. Read it from the repository (raw file on the default branch) or from the catalog API when that exists: both serve the same object at the same `schema_version`.

Readers must ignore fields they do not know. A breaking change bumps `schema_version`; adding a field does not.

## Shape

```json
{
  "schema_version": 1,
  "templates": [ { "id": "lychee", "...": "..." } ],
  "tombstones": [ { "id": "old-id", "replaced_by": "", "reason": "", "removed_in": "v0.8.18" } ]
}
```

## Template fields

| Field | Meaning |
| --- | --- |
| `id`, `aliases` | Stable slug. Renames keep the old id in `aliases`. A tombstoned id is never reused. |
| `name`, `slogan`, `description`, `first_run` | Display text. `first_run` is a list of steps for the first launch. |
| `category` | One catalog category. |
| `status`, `status_reason`, `replaced_by` | `active`, `deprecated` or `yanked`, with a public reason when not active. |
| `source_repo`, `documentation_url` | Upstream links. |
| `license` | `{spdx, class, url}`. Omitted when the entry does not state one. `class` is `permissive`, `copyleft`, `network-copyleft`, `source-available` or `proprietary`. |
| `hosted_use` | `allowed`, `restricted` or `unknown`. For a future hosted service, not shown as a claim. |
| `listing` | `{by, vendor_org, proof}`. `by` is `maintainers`, `community` or `vendor`. A vendor listing means the vendor proved ownership, nothing more. |
| `trust` | `{level, evidence_url}`. `level` is `listed`, `verified` or `verified_signed`. |
| `arch` | `{amd64, arm64}`, each `tested`, `published`, `absent` or `unknown`. |
| `verification` | Present only while a smoke record matches the current pinned images: `{tag, digest, http_status, arches, run_url}`. No dates. |
| `admission` | Present when the admission checks ran: `{kev_hit, secrets_allowed, run_url}`. |
| `images` | One per service: `{service, image, tag, digest, source_class, platforms, signed, provenance, sbom, non_root}`. |
| `memory_mib`, `requires_gpu`, `needs_config` | Sizing and setup hints. |
| `install` | Additive, optional. `{kind, fields}`. `kind` is `install` (a template with a settings form, set up with `levelrail-cli templates install <id> --env KEY=VALUE`) or `deploy` (`levelrail-cli templates deploy <id>`). `fields` is `[{key, label, required, default_kind}]`, empty for `deploy`. `required` means no default and no generator. `default_kind` is `generated`, `preset` or `none`. Values are never exported. |
| `services`, `env`, `requirements` | Ports and volumes per service, environment variable names with a `kind` (never values), and the requirements card. |

## Trust levels

- **Listed**: merged, the loader guards pass, every image has an exact version tag from an allowed namespace, and the admission check found no known-exploited vulnerability and no unallowed secret.
- **Verified**: Listed, plus a recorded passing smoke run for the exact pinned images in `verification`. All services stayed up and the health path answered 200 to 399, or the status the template declares. `verification.arches` lists exactly the architectures that were tested, so a page can say "Verified on arm64". `trust.evidence_url` is the CI run when one is recorded, else the public status page that records the pass.
- **Verified and signed**: Verified, plus a signature verified against an identity in `source_repo` and a provenance attestation on the upstream image.

Verified is a fact about a pinned tag, not a date. When the tag changes, the record stops matching and the entry drops back to Listed until the new tag passes. Records made on a developer machine count, with their architecture disclosed; CI run links are added as the template PR workflow records them.

The admission wording is: no known-exploited vulnerability (CISA KEV) in this image when this version was accepted, with the run linked. It describes an event, not the present state, and the index carries no vulnerability counts.

## Where the data lives

- `internal/catalog/manifests/<id>/`: the template itself.
- `internal/catalog/verified.json`: smoke records, written by the smoke tooling (`id` to `{tag, images, digest, arches, http_status, run_url}`).
- `internal/catalog/images.lock.json`: digests and signals resolved by CI, never written by hand.
- `internal/catalog/platforms.json`: the registry platform scan.

To list your own product, see [Add a template](/add-a-template).
