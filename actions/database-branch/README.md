# Levelrail database branch

Composite action that creates a database branch named `<database>-pr-<number>` for a pull request, posts one comment with its state and a schema diff summary, and deletes the branch when the pull request closes. See the guide at https://docs.levelrail.com/database-branch-per-pull-request for a full workflow.

Inputs: `server`, `token`, `database` (required), `mode` (`schema` or `full`), `ttl`, `pr-number`, `comment`, `version`, `github-token`. Outputs: `branch`, `host`, `port`, `database-name`.
