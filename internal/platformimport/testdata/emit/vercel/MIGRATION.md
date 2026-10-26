# Migration notes for stack vercel-import

Generated from vercel. Review the documents in a pull request, then run `levelrail-cli stack plan vercel-import -f .` before the first apply.

## What the files do not do

- Apply runs prebuilt images only. An app with no image on the source is named after the `--image-repo` you gave, and nothing builds it: push that image from your CI first.
- Secret values, volumes and database contents are not carried over by the documents.
- crons of project web are not read from the API, they come from vercel.json when the repository is imported
- project web: value not readable, set it manually: API_KEY, DB_PASS
- project web: env vars differ by target (production, preview, development), the production value was used: FEATURE (development+production), PREVIEW_ONLY (preview)
- connected Vercel storage of project web is not discoverable here, check it manually: Postgres maps to a Levelrail Postgres database, KV (Redis) to a Levelrail Redis database, Blob needs an S3 compatible bucket, Edge Config has no equivalent (none)
- crons of project docs are not read from the API, they come from vercel.json when the repository is imported
- connected Vercel storage of project docs is not discoverable here, check it manually: Postgres maps to a Levelrail Postgres database, KV (Redis) to a Levelrail Redis database, Blob needs an S3 compatible bucket, Edge Config has no equivalent (none)
- crons of project bare are not read from the API, they come from vercel.json when the repository is imported
- connected Vercel storage of project bare is not discoverable here, check it manually: Postgres maps to a Levelrail Postgres database, KV (Redis) to a Levelrail Redis database, Blob needs an S3 compatible bucket, Edge Config has no equivalent (none)

## App bare

- no port found in the source, defaulted to 80
- Vercel project is not linked to a repository this importer can resolve
- next: set the correct port in the app settings
- next: import the repository with `import <repo>` and attach it to this app

## App docs

- no port found in the source, defaulted to 80
- the app builds from git; it is created without a running build
- Vercel Node.js version is "22.x"
- next: set the correct port in the app settings
- next: connect the repository https://gitlab.com/acme/docs (trunk), set credentials if it is private, then run a build
- next: set the same value in the build settings after import

## App web

- no port found in the source, defaulted to 80
- the app builds from git; it is created without a running build
- secret variables with no readable value were skipped: API_KEY, DB_PASS
- Vercel build command is "pnpm build"
- Vercel install command is "pnpm i"
- Vercel output directory is ".next"
- Vercel Node.js version is "20.x"
- next: set the correct port in the app settings
- next: connect the repository https://github.com/acme/mono (main), set credentials if it is private, then run a build
- next: set them with the secrets command
- next: set the same value in the build settings after import
- next: set the same value in the build settings after import
- next: set the same value in the build settings after import
- next: set the same value in the build settings after import
