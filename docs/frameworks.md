---
title: "Framework presets"
description: "Deploy Next.js, Go, React/Vite, Spring Boot and eight more frameworks with the right port, health path, storage and env hints already filled in."
"og:url": "https://docs.levelrail.com/frameworks"
---

A framework preset is the set of defaults a framework needs to run well: the port it listens on, a health path, how it builds, the environment variables it will not boot without, and the paths worth keeping across deploys. The control plane detects the framework from your repository and fills these in, so a first deploy needs no manual build configuration.

## How it works

When you point the create-app form at a repository, the detector reads the manifest files (`package.json`, `go.mod`, `pom.xml`, `requirements.txt`, `Gemfile`, `composer.json`) and picks a preset. The picker shows the match with a sparkle mark. You can choose a different preset at any time.

If the port or health path in the form differs from the preset, the form shows **Use preset settings**. One click applies the port, health path and build type. Nothing is submitted until you create the app.

From the command line:

```bash
levelrail-cli build frameworks                  # list presets
levelrail-cli build frameworks spring-boot      # port, health path, env hints, volumes
levelrail-cli build frameworks go --dockerfile  # print the tuned Dockerfile
levelrail-cli apps create --name api --repo https://github.com/you/api.git \
  --image-repo registry.example.com/you/api --framework spring-boot
```

`--framework` fills the port, build type and volumes you did not set yourself, and prints a note for any required secret such as `SECRET_KEY_BASE`. The API is `GET /api/v1/frameworks`, `GET /api/v1/frameworks/{id}` and `GET /api/v1/frameworks/{id}/dockerfile`.

## Presets

| Preset | Port | Health path | Build | Output |
| --- | --- | --- | --- | --- |
| Next.js | 3000 | `/` | Railpack | standalone server |
| Next.js (static export) | none | `/` | static | static files |
| React / Vite | none | `/` | static | static files |
| Astro | none | `/` | static | static files |
| Nuxt | 3000 | `/` | Railpack | Nitro server |
| Remix | 3000 | `/` | Railpack | remix-serve |
| SvelteKit | 3000 | `/` | Railpack | adapter-node server |
| Node.js (Express) | 3000 | `/` | Railpack | server |
| Go | 8080 | `/healthz` | Railpack | single binary |
| Spring Boot | 8080 | `/actuator/health` | Railpack | executable jar |
| Django | 8000 | `/` | Railpack | gunicorn |
| FastAPI | 8000 | `/docs` | Railpack | uvicorn |
| Ruby on Rails | 3000 | `/up` | Railpack | puma |
| Laravel | 80 | `/up` | Railpack | FrankenPHP |

Static presets have no port: the built-in web server serves the files directly and no container runs. With the CLI, declare `build.type: static` in `app.yaml` and use `--file`.

## Deploy guides

### Next.js

Set `output: 'standalone'` in `next.config` for the smallest runtime image. The preset mounts a volume at `/app/.next/cache` so ISR pages and optimized images survive a redeploy. `NEXT_PUBLIC_*` values are inlined at build time, so pass them as build args, not runtime env. `levelrail-cli build frameworks nextjs --dockerfile` prints a multi-stage Dockerfile with a non-root user.

### Next.js static export

With `output: 'export'` the detector chooses the static preset. The `out` directory is served directly.

### React / Vite

The `dist` directory is served as a static site. Variables prefixed `VITE_` are baked in at build time. Single page apps fall back to `index.html`.

### Astro

Static output is the default and is served directly. For `output: 'server'` pick Node.js and set the port to 4321.

### Nuxt

The Nitro server listens on 3000. Set `NITRO_HOST=0.0.0.0` if the app is unreachable from the proxy.

### Remix

The preset runs `remix-serve ./build/server/index.js` after `remix vite:build`. Set `SESSION_SECRET` before the first deploy.

### SvelteKit

Use `adapter-node`. `ORIGIN` must match the public URL or form actions fail the CSRF check.

### Node.js (Express)

Listen on `process.env.PORT` and bind `0.0.0.0`. The detector reads a port from `scripts.start` when it finds one.

### Go

The preset expects `/healthz` and a `PORT` variable. `levelrail-cli build frameworks go --dockerfile` prints a static build on a distroless nonroot image, a few megabytes in total.

### Spring Boot

Add `spring-boot-starter-actuator` so `/actuator/health` exists. The preset allows 120 seconds for readiness because a JVM cold start can be slow, and sets `-XX:MaxRAMPercentage=75` so the heap follows the container memory limit. The Dockerfile template builds a layered jar so dependency layers are cached between deploys.

### Django

Set `DJANGO_SECRET_KEY` as a secret and add your domain to `DJANGO_ALLOWED_HOSTS`. A 400 response from a fresh deploy almost always means the host is missing. Uploaded files live under `/app/media`, which the preset persists.

### FastAPI

The default `/docs` page doubles as a health path. Replace it with a real `/health` route when you have one.

### Rails

`SECRET_KEY_BASE` is required. `/up` is the built-in Rails health route. Active Storage disk files live under `/app/storage`, which the preset persists.

### Laravel

`APP_KEY` is required: generate it with `php artisan key:generate --show`. `/up` is the Laravel health route. Uploads and logs live under `/app/storage`, which the preset persists.

## Test fixtures

Each preset has a minimal sample project under `test/fixtures/frameworks/<id>`. The detector tests run against all of them, so a change that stops one from being recognised fails the build.
