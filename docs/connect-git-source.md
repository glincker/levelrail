# Connect a Git source to an existing app

An app deployed from an image, an upload or the CLI can switch to building from a Git repository without being recreated. Env vars, secrets, domains, volumes and replicas stay. The running image keeps serving until the first build succeeds, and it stays in deploy history for rollback.

## Dashboard

Open the app, go to **Source** and press **Connect Git source** (the link `/apps/<name>/source?connect=1` opens the same dialog).

1. **Source.** Pick a repository from a connected provider (GitHub, GitLab, Bitbucket, Gitea) or paste a URL, then pick a branch.
2. **Review.** The dialog reads the repository and shows the detected build, what the app keeps, and every other service of the same app or compose stack with what happens to it. When the app has several services, choose which one this repository builds. The others keep their image or their own Git source unless they are listed as built from this repository.
3. **Finish.** Choose **Deploy now** to start the first build, or **Link only** to wait for the next push. After Deploy now the dialog shows the deploy status, the failing step and the error summary as they arrive on the deploy event stream, plus a link to the deploy.

Apps in a protected environment only offer Link only: push to the branch so the deploy goes through approval. If the control plane cannot read the protection state, Deploy now is refused until it can.

## CLI

```sh
# Preview what changes, nothing is saved
levelrail apps git-source plan web --repo https://github.com/org/web

# Read a private GitLab, Gitea or Bitbucket Server repo with a provider connection
levelrail apps git-source plan web --repo https://gitlab.com/org/private --connection gc_123

# Attach and build now
levelrail apps git-source set web --repo https://github.com/org/web --branch main --deploy

# Build the connected branch again later
levelrail apps git-source deploy web
```

The plan output lists the other services of the app under `also in app`.

## API

- `POST /api/v1/apps/{name}/git-source/plan` with `repo_url`, `branch` and an optional `connection_id`. Returns the current source, what is kept, the suggestion, warnings and `group` (sibling services with `effect`: `unchanged`, `keeps_own_source` or `built_with_this_source`).
- `PUT /api/v1/apps/{name}/git-source` attaches the repository.
- `POST /api/v1/apps/{name}/git-source/deploy` builds the connected branch now.

## Access rules

- A provider connection token is used to read a repository only when the caller can read sensitive values, and only for an https repository URL without credentials on that connection's own host and port. Bitbucket Cloud repositories are not read before connecting yet, so their build is picked after the connect. A deploy-only token gets the plan without it, with the warning `connection_needs_read_sensitive`.
- Deploy now keeps the deploy freeze gate, the per-app build lock and the node safety checks of every other deploy.
