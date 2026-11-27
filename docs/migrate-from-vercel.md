---
title: "Migrate a project from Vercel"
description: "Move one Vercel project to Levelrail with levelrail-cli import vercel: a dry run first, seven printed steps, domains attached, DNS left for you, and a rollback path."
"og:url": "https://docs.levelrail.com/migrate-from-vercel"
---

Outcome: one Vercel project running on Levelrail under its own domain, with the original left untouched. `levelrail-cli import vercel [dir]` needs only your Levelrail credentials. It never changes DNS and never deletes or edits anything on Vercel.

## What moves

| Item | What happens |
| --- | --- |
| Framework, build and output | Read from `vercel.json` and `package.json`, then deployed like `levelrail-cli deploy` |
| Environment variables | Values come from `--env-file` or the shell and are never printed |
| Domains | Each `--domain` is attached (apex and `www`, `www` redirects to the apex). No DNS records are written |
| Crons | Each entry becomes an HTTP job on the app |
| Redirects, rewrites, headers | Translated where supported, the rest is listed |
| Regions, functions | Reported as not applicable on a self-hosted server |

## Steps

<Steps>
<Step title="Dry run">

Every step is printed and nothing is changed.

```sh
levelrail-cli import vercel ./my-site --env-file .env.production --domain example.com --dry-run
```

</Step>
<Step title="Run it">

```sh
levelrail-cli import vercel ./my-site --env-file .env.production --domain example.com --yes
```

</Step>
<Step title="Verify">

Test the temporary URL of the app before touching DNS.

</Step>
<Step title="Cut over DNS">

Lower the TTL a day ahead, then change the records at your registrar to the ones the command printed.

</Step>
<Step title="Roll back if needed">

Run `levelrail-cli apps rollback APP` or point DNS back at Vercel.

</Step>
</Steps>

## The seven printed steps

1. Read `vercel.json` and `package.json`: framework, build and output, rewrites, redirects, headers and crons. Regions are ignored with a note.
2. List the environment variables. `NEXT_PUBLIC_` and `VITE_` keys are build time, so they are sent with `--env` on the upload deploy. A file whose name looks like a secret store is refused unless you pass `--allow-secret-path`.
3. Deploy the directory with the same path as `levelrail-cli deploy`.
4. Attach each `--domain`.
5. Run the domain cutover check and print the exact DNS records to set.
6. Translate redirects and headers where supported and list the rest. Each `crons` entry becomes an HTTP job (a GET sending `Authorization: Bearer` from the `CRON_SECRET` app env var; the value is never printed). Existing jobs with the same name are kept. For Next.js projects, redirects, rewrites and headers are left to `next.config`; for other projects each rule is listed as not carried over. `regions` and `functions` are reported as not applicable on a self-hosted server.
7. Print the final checklist.

## Not translated

Edge middleware, rewrites with `has` or `missing` conditions, image optimization settings, and hosted analytics. The command lists each one it finds.

<Note>
The Vercel project, its domains, env vars and integrations stay as they were. Remove them yourself once traffic is confirmed. For a Next.js runbook with concept mapping and a shadow run, see [Migrating a Next.js app from Vercel](/migrating-from-vercel).
</Note>

<AccordionGroup>
  <Accordion title="The command refuses my env file">
    A file whose name looks like a secret store is refused. Pass `--allow-secret-path` if you mean to use it.
  </Accordion>
  <Accordion title="My redirects or headers did not carry over">
    Next.js projects keep these in `next.config`. For other projects each rule that was not carried over is listed in the output, so recreate it on the app.
  </Accordion>
</AccordionGroup>
