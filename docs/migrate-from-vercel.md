# Migrate a project from Vercel

`levelrail-cli import vercel [dir]` moves one project in seven printed steps.
It needs only your Levelrail credentials. It never changes DNS and never
deletes or edits anything on Vercel.

```sh
levelrail-cli import vercel ./my-site --env-file .env.production --domain example.com --dry-run
levelrail-cli import vercel ./my-site --env-file .env.production --domain example.com --yes
```

Run with `--dry-run` first: every step is printed and nothing is changed.

## The steps

1. Read `vercel.json` and `package.json`: framework, build and output, rewrites, redirects, headers and crons. Regions are ignored with a note.
2. List the environment variables. Values come from `--env-file` or the shell environment and are never printed. `NEXT_PUBLIC_` and `VITE_` keys are build time, so they are sent with `--env` on the upload deploy. A file whose name looks like a secret store is refused unless you pass `--allow-secret-path`.
3. Deploy the directory with the same path as `levelrail-cli deploy`.
4. Attach each `--domain`: apex and `www` are both attached, `www` redirects to the apex. No DNS records are written.
5. Run the domain cutover check and print the exact DNS records to set.
6. Translate redirects and headers where supported and list the rest.
7. Print the final checklist.

## After the command

- Test the temporary URL of the app.
- Change the DNS records at your registrar. Lower the TTL a day ahead.
- To go back, run `levelrail-cli apps rollback APP` or point DNS back at Vercel.
- The Vercel project, its domains, env vars and integrations stay as they were. Remove them yourself once traffic is confirmed.

Not translated: edge middleware, rewrites with `has` or `missing` conditions, image optimization settings, and hosted analytics. The command lists each one it finds.
