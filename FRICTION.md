# Migration friction log

Track every point where moving a real app onto Levelrail was blocked, confusing, or rough. Filled in during the Vercel migration described in [docs/migrating-from-vercel.md](docs/migrating-from-vercel.md).

## How to use

- Add one row per problem, at the moment you hit it, before you work around it.
- ID: `F-001`, `F-002`, and so on, in order.
- Step: the runbook step or command you were on.
- What happened: the observed behavior, and the expected behavior if it differs. Include exact error text when short.
- Severity:
  - **S1**: blocks the migration.
  - **S2**: confusing, or needs a manual workaround.
  - **S3**: polish (wording, ordering, a missing hint).
- Fix task: a link to the issue or branch that fixes it, or a one-line description if none exists yet.
- Status: `open`, `in progress`, or `fixed`. Add the PR link when fixed.
- Never paste secrets, tokens, or real customer data into a row.

| ID | Step | What happened | Severity | Fix task | Status |
| --- | --- | --- | --- | --- | --- |
