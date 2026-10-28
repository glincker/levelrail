---
title: "Use any CI"
description: Build with GitLab CI, CircleCI, Jenkins, Buildkite or any other CI, and let Levelrail host. Templates, exit codes and safe retries.
"og:url": "https://docs.levelrail.com/use-any-ci"
---

You do not need to build on Levelrail. Your CI builds and pushes an image to a registry, then tells Levelrail to deploy it and waits for the result. Levelrail only hosts.

```text
your CI: build image, push to registry  ->  levelrail-cli apps deploy --image  ->  levelrail-cli apps wait
```

## The three steps

1. **Install the CLI** in the job, or call the [API](/api-reference) directly. The installer is `curl -fsSL https://levelrail.com/install-cli.sh | sh`. Set `LEVELRAIL_CLI_INSTALL_DIR` to a directory you can write to (the default is `/usr/local/bin` for root and `~/.local/bin` otherwise) and `LEVELRAIL_VERSION=vX.Y.Z` to pin a release.
2. **Authenticate with environment variables only.** `LEVELRAIL_API_URL` is your control plane URL and `LEVELRAIL_TOKEN` is an API token with at least the `deploy` ability. `LEVELRAIL_PROFILE` picks a saved profile and is not needed in CI. The CLI also reads `CI=true` (or `LEVELRAIL_CI=true`) and then never prompts, and it does so by itself when stdin or stdout is not a terminal.
3. **Deploy by image, then wait.** `apps deploy` returns as soon as the deploy is accepted. `apps wait` is what tells you whether the rollout worked.

```bash
levelrail-cli apps deploy my-app --image registry.example.com/team/my-app:"$TAG"
levelrail-cli apps wait my-app --timeout 5m
```

`apps deploy` requires `--image` here; add `--confirm` to deploy into a protected environment. `apps wait` also takes `--attempt-id` and `--poll-interval` (default `3s`).

Create the token once and store it in your CI's secret store:

```bash
levelrail-cli tokens create --name ci --preset deployer --expires-in-days 90
```

## Exit codes

Branch on these in scripts.

| Code | Meaning |
| --- | --- |
| 0 | Success |
| 1 | Usage error: unknown command, missing argument, bad flag |
| 2 | Valid syntax, but the request is invalid |
| 3 | Could not reach the API (connection refused, DNS, timeout) |
| 4 | The API answered with an error status (401, 403, 409, 429 and so on) |
| 5 | `apps wait`: the deploy converged as a failure |
| 6 | `apps wait`: gave up before the deploy converged |
| 7 | `go-live --wait`: the domain is not live yet (not used by deploys) |

A job running `apps deploy` then `apps wait` fails on any non-zero code, which is what you want.

## Safe retries

`apps deploy` sends an `Idempotency-Key`, so re-running the same job applies the deploy once and replays the first result. The CLI derives the key from the app, the image and the run id. It reads only two sets of run variables: `GITHUB_RUN_ID` with `GITHUB_RUN_ATTEMPT`, and `CI_PIPELINE_ID` (set by GitLab CI). On any other CI it falls back to the current minute, so pass your own key from the CI's build id:

```bash
levelrail-cli apps deploy my-app --image "$IMAGE" --idempotency-key "ci-$BUILD_ID"
```

The CLI retries HTTP 429 and 503 for requests that carry an idempotency key, waiting for `Retry-After`. `LEVELRAIL_RETRY_MAX` (default `3`, `0` turns retry off) and `LEVELRAIL_RETRY_MAX_WAIT` (default `60s`) bound it. See [Deploy from your computer](/deploy-from-cli#safe-retries-in-ci).

## Ready-to-copy pipelines

Each file builds an image, pushes it, deploys it pinned by digest and waits for the rollout to be healthy. A test in the CLI parses every file and fails if a command or flag it uses no longer exists, so they stay current.

| CI | File | Auth |
| --- | --- | --- |
| GitHub Actions | [ci-examples/github-actions-oidc.yml](https://github.com/glincker/levelrail/blob/main/docs/ci-examples/github-actions-oidc.yml) | OIDC, no stored Levelrail secret |
| GitLab CI | [ci-examples/gitlab-ci.yml](https://github.com/glincker/levelrail/blob/main/docs/ci-examples/gitlab-ci.yml) | API token in a masked variable |
| CircleCI | [ci-examples/circleci.yml](https://github.com/glincker/levelrail/blob/main/docs/ci-examples/circleci.yml) | API token in a restricted context |

The GitHub file needs a CI trust on the app once: `levelrail-cli apps github-actions trust add my-app --repo example/my-app --ref refs/heads/main`. The job then runs `levelrail-cli apps github-actions token my-app`, which swaps the job's identity token for a deploy token that expires after 15 minutes.

## Deploy by digest from a build-complete hook

If your CI already built and pushed the image and you would rather call the API than install the CLI, send the deploy straight to the same endpoint `apps deploy` uses. There is no separate webhook to set up:

```bash
curl -fsS -X POST "$LEVELRAIL_API_URL/api/v1/apps/my-app/deploys" \
  -H "Authorization: Bearer $LEVELRAIL_TOKEN" \
  -H "Idempotency-Key: ci-$BUILD_ID" \
  -H "Content-Type: application/json" \
  -d '{"image": "registry.example.com/team/my-app@sha256:<digest>"}'
```

- `image` takes `name:tag`, `name@sha256:digest` or `name:tag@sha256:digest`. Pin by digest so a re-pushed tag can never change what runs.
- The token needs the `deploy` ability on the app, the same as the CLI. An OIDC exchange token from GitHub works here too.
- The `Idempotency-Key` makes a retried hook apply once: a repeat with the same key and body within 24 hours replays the first response, and the same key with a different body is refused.
- Deploy freezes, protected environment approvals (add `"confirm": true`), deploy policies and rate limits apply exactly as they do to any other deploy, and the call is recorded in the audit log.
- A `202` means accepted. Poll `GET /api/v1/apps/my-app/deploy-attempts`, or run `levelrail-cli apps wait my-app`, to learn whether it became healthy.

The CLI equivalent is `levelrail-cli apps deploy my-app --image registry.example.com/team/my-app@sha256:<digest>`.

## OIDC support

[Deploy without a stored secret](/github-actions#deploy-without-a-stored-secret-oidc) works on GitHub Actions only. The control plane accepts identity tokens only from `https://token.actions.githubusercontent.com` and matches them by GitHub repository, ref and environment (`internal/api/ci_oidc.go`). GitLab ID tokens, CircleCI OIDC and Buildkite OIDC are not accepted today, so the templates below use an API token. Supporting another CI would need a verifier for that issuer and a mapping from its claims to repository, ref and environment.

## GitLab CI

Store `LEVELRAIL_API_URL` and `LEVELRAIL_TOKEN` under **Settings > CI/CD > Variables**, masked, and protected if you deploy only from protected branches.

```yaml
deploy:
  stage: deploy
  image: docker:27
  services: [docker:27-dind]
  rules:
    - if: $CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH
  variables:
    IMAGE: $CI_REGISTRY_IMAGE:$CI_COMMIT_SHA
    LEVELRAIL_CLI_INSTALL_DIR: /usr/local/bin
  script:
    - docker login -u "$CI_REGISTRY_USER" -p "$CI_REGISTRY_PASSWORD" "$CI_REGISTRY"
    - docker build -t "$IMAGE" .
    - docker push "$IMAGE"
    - apk add --no-cache curl
    - curl -fsSL https://levelrail.com/install-cli.sh | sh
    - levelrail-cli apps deploy my-app --image "$IMAGE"
    - levelrail-cli apps wait my-app --timeout 5m
```

`CI_PIPELINE_ID` is read for the idempotency key, so no extra flag is needed. The control plane must be able to pull `$IMAGE`, so add the registry credentials to the app first.

## CircleCI

Store both variables in **Project Settings > Environment Variables** or in a context.

```yaml
version: 2.1
jobs:
  deploy:
    docker:
      - image: cimg/base:stable
    environment:
      LEVELRAIL_CLI_INSTALL_DIR: /home/circleci/bin
    steps:
      - checkout
      - setup_remote_docker
      - run:
          name: Build and push
          command: |
            echo "$REGISTRY_PASSWORD" | docker login ghcr.io -u "$REGISTRY_USER" --password-stdin
            docker build -t "ghcr.io/example/my-app:$CIRCLE_SHA1" .
            docker push "ghcr.io/example/my-app:$CIRCLE_SHA1"
      - run:
          name: Deploy to Levelrail
          command: |
            curl -fsSL https://levelrail.com/install-cli.sh | sh
            export PATH="$LEVELRAIL_CLI_INSTALL_DIR:$PATH"
            levelrail-cli apps deploy my-app \
              --image "ghcr.io/example/my-app:$CIRCLE_SHA1" \
              --idempotency-key "circle-$CIRCLE_WORKFLOW_ID"
            levelrail-cli apps wait my-app --timeout 5m
workflows:
  main:
    jobs:
      - deploy:
          filters:
            branches:
              only: main
```

## Jenkins

Create two **Secret text** credentials with the ids `levelrail-api-url` and `levelrail-token` under **Manage Jenkins > Credentials**.

```groovy
pipeline {
  agent any
  environment {
    IMAGE = "registry.example.com/team/my-app:${env.GIT_COMMIT}"
    LEVELRAIL_API_URL = credentials('levelrail-api-url')
    LEVELRAIL_TOKEN = credentials('levelrail-token')
    LEVELRAIL_CLI_INSTALL_DIR = "${env.WORKSPACE}/.bin"
    CI = 'true'
  }
  stages {
    stage('Build and push') {
      steps {
        sh 'docker build -t "$IMAGE" .'
        sh 'docker push "$IMAGE"'
      }
    }
    stage('Deploy') {
      steps {
        sh 'curl -fsSL https://levelrail.com/install-cli.sh | sh'
        sh '''
          export PATH="$LEVELRAIL_CLI_INSTALL_DIR:$PATH"
          levelrail-cli apps deploy my-app --image "$IMAGE" --idempotency-key "jenkins-$BUILD_TAG"
          levelrail-cli apps wait my-app --timeout 5m
        '''
      }
    }
  }
}
```

The single-quoted `sh` strings keep Groovy from interpolating the secrets into the command line, and Jenkins masks the credential values in the log.

## Buildkite

Keep the secrets in your agent environment hook, a secrets manager plugin, or **Pipeline Settings**. Do not put them in `pipeline.yml`.

```yaml
steps:
  - label: "Build and push"
    command: |
      docker build -t "registry.example.com/team/my-app:$BUILDKITE_COMMIT" .
      docker push "registry.example.com/team/my-app:$BUILDKITE_COMMIT"
  - wait
  - label: "Deploy to Levelrail"
    branches: "main"
    env:
      LEVELRAIL_CLI_INSTALL_DIR: "$BUILDKITE_BUILD_CHECKOUT_PATH/.bin"
    command: |
      curl -fsSL https://levelrail.com/install-cli.sh | sh
      export PATH="$$LEVELRAIL_CLI_INSTALL_DIR:$$PATH"
      levelrail-cli apps deploy my-app \
        --image "registry.example.com/team/my-app:$BUILDKITE_COMMIT" \
        --idempotency-key "buildkite-$BUILDKITE_BUILD_ID"
      levelrail-cli apps wait my-app --timeout 5m
```

`$$` stops Buildkite from expanding the variable at upload time. `LEVELRAIL_API_URL` and `LEVELRAIL_TOKEN` must already be in the step's environment.

## Troubleshooting

| Symptom | Likely cause and fix |
| --- | --- |
| Exit 4, `401` | The token is missing, wrong, revoked or expired. Check `LEVELRAIL_TOKEN` reaches the step (masked or protected variables are not exposed to unprotected branches). |
| Exit 4, `403` | The token lacks the `deploy` ability for this app, or an IAM policy denies it. Create it with `--preset deployer`. |
| Exit 4, `409` on deploy | The app is in a protected environment. Add `--confirm` to `apps deploy`. |
| Exit 4, `429` | Rate limited. The CLI already retries with the idempotency key; raise `LEVELRAIL_RETRY_MAX` or `LEVELRAIL_RETRY_MAX_WAIT` if it runs out. |
| Exit 3 | The runner cannot reach `LEVELRAIL_API_URL`. Check DNS, firewall and the URL. |
| Exit 5 | The deploy ran and failed. Read the app's deploy log, or see [Deploy failures](/deploy-failures). |
| Exit 6 | `apps wait` hit `--timeout` first. The deploy may still finish; raise `--timeout` or check the app. |
| Image cannot be pulled | The control plane needs registry credentials for a private registry. |

## Next steps

- [Deploying from GitHub Actions](/github-actions): the OIDC, no stored secret path.
- [CLI reference](/cli-reference): `apps deploy` and `apps wait` in full.
- [Identity and access](/identity-and-access): token abilities and presets.
