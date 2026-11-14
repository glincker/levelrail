# Registry release plan (mirror repository)

Status: configuration only. Nothing is published, no key exists, no tag or release has been created.

The Terraform and OpenTofu registries need a repository named `terraform-provider-<name>` with the provider at its root. This monorepo cannot be that, so the provider is mirrored into `glincker/terraform-provider-levelrail`. The registry address is `glincker/levelrail`; the npm scope for related packages is `@levelrail`.

## Files

- `../.goreleaser.yml`: builds linux, darwin, windows and freebsd (amd64, arm64; no windows/arm64), CGO off, `-trimpath`, version injected into `main.version`, zip archives, a SHA256SUMS file signed with GPG, draft releases, no changelog (release-please owns it).
- `../terraform-registry-manifest.json`: protocol 6.0 (the provider uses terraform-plugin-framework and serves protocol 6).
- `mirror-sync.sh`: copies the provider files into a clone of the mirror and tags it. Dry run by default, `--yes` to act, refuses any origin except the mirror repo, copies no history and no other directory.
- `workflow.yml.example`: the release workflow for the mirror. It lives here as a template; the monorepo workflows are not touched.

## One time owner steps

1. Create the public repository `glincker/terraform-provider-levelrail`.
2. Create a GPG key for signing (RSA or ECC, no expiry or a long one). Keep the private key offline.
3. Upload the public key to the Terraform registry under the `glincker` namespace (Settings, Signing Keys).
4. Claim the `glincker` namespace on the registry and publish the provider from the mirror repo (this installs the registry webhook).
5. Add the repository secrets `GPG_PRIVATE_KEY` and `PASSPHRASE` to the mirror repo.
6. Copy the workflow: `mirror-sync.sh` installs `workflow.yml.example` as `.github/workflows/release.yml` in the mirror.

## Release checklist

1. Bump the version through the release-please trailer in the monorepo release flow; note the provider version.
2. Clone the mirror and dry run: `release/mirror-sync.sh --clone <path> --version X.Y.Z`.
3. Read the output, then run it again with `--yes`. It commits, tags `vX.Y.Z` and pushes to the mirror.
4. The tag triggers the workflow. goreleaser builds and signs, and leaves a draft GitHub release.
5. Review the draft, publish it. The registry webhook picks up the new version.
6. Check `terraform init` from a clean directory against the registry.

## Docs generation

`tfplugindocs` is not a dependency of the module today, so nothing generates a `docs/` directory and `docs/terraform-provider.md` in the monorepo stays the source. To add it later (this edits go.mod, so ask first):

```bash
go get -tool github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs
go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs generate
```

## Not validated

`goreleaser` was not installed when this was written, so `goreleaser check` has not run. Run it once from a mirror clone before the first tag.
