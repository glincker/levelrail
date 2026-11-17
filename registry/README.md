# Template registry

One folder per template under `registry/templates/<id>/`. The folder name is the template id.

```
registry/templates/<id>/
  template.yaml          metadata (see fields below)
  compose.yaml           the compose body, kept byte for byte
  manifest.compose.yaml  only for templates with a manifest whose raw compose differs
```

## Fields in template.yaml

- `id`, `name`, `slogan`: required.
- `description`: optional setup notes (ports, firewall, required env, licence), plain text.
- `category`, `documentation_url`.
- `recommended_memory_bytes`: advisory RAM, 0 means none.
- `requires_gpu`: true when the compose reserves an NVIDIA GPU.
- `platforms`: os/arch pairs verified in the image manifests, empty means not checked.
- `icon`: optional thesvg slug, empty for now.
- `manifest`: the install manifest (env prompts, domains, healthcheck) when the template has one.

## Add a template

Drop a new folder with `template.yaml` and `compose.yaml`, then open a PR. CI will validate it.

## Today

The Go catalog is still what the binary serves. This folder is generated from it:

```
go run ./scripts/export-registry          # write
go run ./scripts/export-registry -check   # fail if stale
```

`LoadRegistryDir` in `internal/catalog` reads this folder back, and a test proves it equals the catalog field for field.
