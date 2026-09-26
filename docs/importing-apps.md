---
description: Import an app from a repo URL, a docker run command, an image, a compose file or a Dockerfile, review the deployment plan, then deploy it.
---

# Importing apps

The "New app" dialog opens with one input, "Import anything". Paste or upload what you already have and Levelrail classifies it and shows a deployment plan preview before anything is created. The same flow is available from the CLI (`levelrail-cli import`), the HTTP API (`POST /api/v1/imports/plan`) and the MCP server (`plan_import`).

A plan is a preview only. Deploying it uses the existing create, build and compose endpoints; there is no separate deploy path.

## What you can paste

| Input | Example | What the plan does |
| --- | --- | --- |
| Repo URL | `https://github.com/owner/repo`, `gitlab.com/group/sub/repo`, a Gitea or Bitbucket URL | Reads a few files from the repo and picks a build method, port and health path. Deploys through the build endpoint. |
| `docker run` command | `docker run -d -p 8080:80 -v data:/data nginx:1.27` | Maps the flags onto an app and lists every flag it cannot honour. |
| Image reference | `ghcr.io/owner/app:1.0.0` | Deploys the image as is. Warns when the tag is `latest` or missing. |
| Compose file | pasted or uploaded `docker-compose.yml` | One plan per service, validated with the same rules as the Docker Compose deploy. |
| Dockerfile | pasted or uploaded | Extracts `EXPOSE` and `ENV`. A Dockerfile alone has no build context, so the plan cannot be deployed until it lives in a repo. |

Classification is automatic. The web UI shows a "Looks like" hint while you type, and the server's decision is final.

## Repository detection

For a repo URL the plan reads a small, fixed set of files over HTTPS from the host's raw-file endpoint (GitHub, GitLab including subgroups, Bitbucket, Gitea and similar). Nothing is cloned to disk and nothing from the repo is executed. Private repos cannot be read this way; connect them through a git provider first.

Detection order:

1. A compose file (`docker-compose.yml`, `docker-compose.yaml`, `compose.yaml`, `compose.yml`) whose services are all prebuilt images is planned as a compose deploy. A compose file that builds from source falls through to the next step with a warning.
2. A root `Dockerfile` is planned as a Dockerfile build. The port comes from `EXPOSE`.
3. Manifest files (`package.json`, `go.mod`, `requirements.txt`, `pyproject.toml`, `Pipfile`, `pom.xml`, `build.gradle`, `index.html`) pick a Railpack build or a static site, with framework defaults for the port and a health check path guess (Next.js, Nuxt, NestJS, Express, Django, FastAPI, Flask, Spring and others).
4. If nothing matches and the repo is on a public host, Railpack's own provider detection is tried.

The plan states why it chose the build method.

Limits are set with environment variables on the control plane:

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_IMPORT_MAX_FILE_BYTES` | `524288` | Largest single file read from a repo |
| `APP_IMPORT_MAX_FILES` | `32` | Files read per plan |
| `APP_IMPORT_FETCH_TIMEOUT` | `20s` | Per-request timeout |

Outbound requests use the same SSRF guard as webhooks: loopback, private and link-local addresses are refused unless `APP_NOTIFY_ALLOW_PRIVATE_NETWORKS=true`. Only `http` and `https` repo URLs are accepted.

## Environment variables

For repos the plan scans `.env.example`, `.env.sample`, `.env.template`, `env.example`, `.env.dist`, compose `environment:` sections and Dockerfile `ENV`. Each variable is reported as:

- **required**: the value is empty or a placeholder such as `changeme`, `your_..._here`, `<...>` or `${VAR}`. Deploy is blocked until it has a value.
- **has default**: a real-looking default exists.
- **secret**: the name contains `SECRET`, `TOKEN`, `PASSWORD`, `KEY` and similar, or the value is a URL with credentials.

The plan never echoes a secret-looking default back. Secret values you enter are stored as encrypted secrets, not plain env vars. A `-e NAME=value` in a `docker run` command that looks secret is treated as required so you re-enter it.

## Docker run flags

Supported: `-p/--publish` (host and container ports, optional IP and protocol), `-e/--env`, `-v/--volume` (named volumes and bind mounts), `--restart`, `--name`, `-m/--memory`, `--cpus`, `--entrypoint`, the image and its command, and `--network` (only noted). The command is tokenized (quotes, backslash escapes, backslash-newline continuations) and never run through a shell.

Some inputs need care:

- A host bind mount (`-v /srv/data:/data`) is flagged "needs root approval", because only root may create one.
- Mounting `/var/run/docker.sock` is flagged, since it gives the container control of the host.
- `-e NAME` with no value takes its value from your shell in Docker; enter it in the plan instead.
- `--env-file` cannot be read from here; paste its variables into the env table.
- Only the first `-p` port is routed through the ingress; other published ports stay on the host.

Flags that are not supported are listed as warnings and ignored, never dropped silently:

| Flag | Note |
| --- | --- |
| `--privileged`, `--cap-add`, `--cap-drop`, `--device`, `--security-opt` | privilege and device changes |
| `--pid`, `--ipc`, `--uts`, `--userns`, `--cgroupns` | shared namespaces (`--pid host` is called out explicitly) |
| `--network host` | host networking; ports are published instead |
| `--sysctl`, `--ulimit`, `--tmpfs`, `--mount`, `--shm-size`, `--read-only`, `--init` | runtime tuning |
| `--link`, `--volumes-from`, `--add-host`, `--dns`, `--dns-search`, `--dns-option`, `--ip`, `--mac-address`, `--network-alias` | legacy links and network detail |
| `--user`/`-u`, `--workdir`/`-w`, `--hostname`/`-h`, `--group-add`, `--stop-signal`, `--stop-timeout` | process settings |
| `--log-driver`, `--log-opt`, `--label`/`-l`, `--label-file`, `--annotation`, `--cidfile` | metadata and logging |
| `--gpus` | set GPUs on the app after it exists |
| `--health-cmd` and other health flags | add an HTTP health check on the app afterwards |
| `--runtime`, `--platform`, `--cpuset-cpus`, `--memory-swap`, `--pids-limit`, `--cpu-shares`, `--oom-score-adj`, `--kernel-memory`, `--storage-opt` | runtime and resource tuning |

## From the dashboard

Open New app, paste into "Import anything" (or upload a compose file or Dockerfile), and choose Preview plan. The plan shows the source, the build method with its reason, an editable app name and container port, an env table with Required and Secret indicators and inline value inputs, volumes, a domain suggestion and warnings. Deploy stays disabled until every required variable has a value. Deploying a repo opens the live build log. The template, image, git and compose cards below the input remain available.

## From the CLI

```bash
# preview only, creates nothing
levelrail-cli import https://github.com/owner/repo
levelrail-cli import ghcr.io/owner/app:1.0.0
levelrail-cli import -f docker-compose.yml
levelrail-cli import --docker-run "docker run -d -p 5432:5432 -e POSTGRES_PASSWORD=x postgres:16"

# create and deploy; required variables come from --env, or are prompted for on a terminal
levelrail-cli import https://github.com/owner/repo --deploy --name site --env API_TOKEN=abc

# machine readable plan
levelrail-cli import nginx:1.27 --json
```

When required variables have no value and no terminal is attached, `--deploy` fails and lists them. Other flags: `--port`, `--ref`.

## From the API and MCP

`POST /api/v1/imports/plan` needs the write ability and returns the plan JSON. The body is `{"text": "...", "kind": "repo|docker_run|image|compose|dockerfile", "ref": "", "name": "", "port": 0, "env": {}}`; only `text` is required. Supplying `env` values satisfies required variables and injects them into a generated compose file. The `plan_import` MCP tool wraps the same call, is read-only, and its result carries repository text that should be treated as untrusted data.
