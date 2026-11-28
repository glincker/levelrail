---
title: "Docker Hub connection"
description: Save a Docker Hub token once, check that it works, see how many pulls you have left and browse tags of private repositories.
"og:url": "https://docs.levelrail.com/docker-hub-connection"
---

A Docker Hub connection is a registry credential for `docker.io` (user name plus a personal or organization access token). The token is stored encrypted, is never returned by any API response and never appears in logs or errors. Only a root operator can save, rotate or remove it.

## What it gives you

- **Token status**: valid or rejected, the kind of token, the scopes Docker Hub granted and when it was last checked.
- **Pull limits**: the limit, what remains, the window and whether Docker Hub counted you by account or by IP address. Anonymous pulls share a limit per IP address, a token raises it.
- **Tag browser**: tags of public and private repositories, read with the token.
- **Doctor**: a rejected token or a low pull limit shows up in `doctor`, with an explanation.

## Commands

```sh
levelrail registry-credentials create --name hub --registry-host docker.io --username me --password "$TOKEN"
levelrail registry-credentials status <id> -o json
levelrail registry-credentials limits <id> --refresh
levelrail registry-credentials tags <id> library/nginx
levelrail registry-credentials delete <id>
```

`status` and `limits` reuse a recent reading unless you pass `--refresh`.

## API

- `GET /api/v1/registry-credentials/{id}/dockerhub`: token status and limits.
- `GET /api/v1/registry-credentials/{id}/limits`: pull limit only.
- `GET /api/v1/registry-credentials/{id}/tags?repository=library/nginx`: tags.
- `POST /api/v1/registry-credentials/dockerhub/test`: check a token before saving it.

## Settings

Applied live, the environment variable only seeds the default:

- `registry.dockerhub.limit_warn_percent`: warn below this percent of the limit.
- `registry.dockerhub.limits_cache_seconds`: how long a reading is reused.
- `registry.dockerhub.tag_page_size`: tags or repositories per request.
- `registry.dockerhub.use_credential`: use the token for `docker.io` pulls.

## Verify it

Run `levelrail registry-credentials status <id>`. A valid token prints its scopes and the pull limit. Run `levelrail doctor` to see the same checks with fixes. See [private registries](/private-registries#docker-hub) for the scopes to grant.
