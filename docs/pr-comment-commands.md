---
title: "Pull request comment commands"
description: Redeploy, roll back or check an app from a GitHub pull request comment, limited to users with write access, off by default, with the same freeze, approval and policy checks as the dashboard.
"og:url": "https://docs.levelrail.com/pr-comment-commands"
---

A user with write access to the repository can type a command in a pull request comment and Levelrail runs it for the app connected to that repository. The feature is off for every app until you turn it on.

Commands are GitHub only for now. They need the GitHub App connection, because Levelrail asks GitHub who the commenter is.

## Commands

A command is a comment line that starts with the command at the first column, outside a code block. Case does not matter. Anything else on the line makes it a normal comment.

| Command | What it does |
| --- | --- |
| `/levelrail redeploy` | Redeploys the app's tracked branch. It deploys the branch tip, not the pull request's code. |
| `/levelrail rollback` | Rolls back to the newest earlier successful deploy of a different image. |
| `/levelrail status` | Replies with the running image, the latest deploy and any freeze. Changes nothing. |
| `/levelrail preview redeploy` | Rebuilds this pull request's preview environment. Needs previews on. |

Send one command per comment. A comment with two commands runs neither. There is no way to name an app in the comment: the app is the one whose webhook received the event.

## Turn it on

<Tabs>
<Tab title="UI">

Open the app, then **Previews**, and use **Pull request commands**. A second switch allows commands on pull requests from forks and stays off unless you enable it.

</Tab>
<Tab title="CLI">

```bash
levelrail-cli apps set web --pr-comment-commands on
levelrail-cli apps set web --pr-comment-commands-forks on   # optional, see the threat model
levelrail-cli apps set web --pr-comment-commands off
```

</Tab>
<Tab title="API">

```bash
curl -X PUT "$APP_API_URL/api/v1/apps/web/preview-settings" \
  -H "Authorization: Bearer $APP_API_TOKEN" \
  -d '{"pr_comment_commands": true}'
```

`pr_comment_commands_forks` works the same way. Both are `false` by default and are also returned by `GET /api/v1/apps/{name}/git-source`. Changing them needs the sensitive write ability and adds an entry to the app timeline.

</Tab>
</Tabs>

The webhook on the repository must also deliver the **Issue comments** event. Add it in the repository's webhook settings next to the push and pull request events. Levelrail does not change an existing webhook for you.

## Environment

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_PR_COMMENT_COMMANDS_DISABLED` | unset | Kill switch. Any true value, or a value that does not parse, turns the feature off for every app. |
| `APP_PR_COMMENT_COMMANDS_PER_PR_PER_MINUTE` | 5 | Accepted commands per pull request per minute. |
| `APP_PR_COMMENT_COMMANDS_PER_USER_PER_MINUTE` | 10 | Accepted commands per GitHub user per minute. |
| `APP_PR_COMMENT_PERMISSION_CACHE` | 1m | How long a positive permission answer is reused. |
| `APP_PR_COMMENT_LOG_RETENTION` | 720h | How long handled comment ids are remembered. |
| `APP_PR_COMMENT_RUN_TIMEOUT` | 30m | Upper bound for one command's deploy work. |

## What happens on a comment

1. The webhook signature is verified with the same per-app secret as push deploys. An unsigned or wrongly signed delivery is rejected with 401.
2. The kill switch and the app setting are checked. Off means the event is ignored.
3. Only a newly created comment on a pull request counts. Edits, deletes and plain issues are ignored.
4. Comments from bots, from GitHub Apps, and comments carrying Levelrail's own reply marker are ignored.
5. The comment id is recorded. A redelivered webhook, or an operator replay, finds it already handled and does nothing.
6. The per-user and per-pull-request limits are applied before any call to GitHub.
7. Levelrail asks GitHub for the commenter's permission with the GitHub App token. Only `write` and `admin` pass. Any error, including a timeout or a 403, refuses the command. A commenter who fails this check gets no reply.
8. Levelrail reads the pull request from GitHub, not from the webhook body. It must be open, and its head repository must be the connected repository unless fork commands are on.
9. The command runs through the same code as the dashboard and records the trigger as `pr_comment` with the GitHub login as the actor, on the deploy attempt where one is created and always on the app timeline.
10. One short comment is posted with the outcome.

## Threat model

| Threat | Control |
| --- | --- |
| A forged webhook triggers a deploy | HMAC signature verification with the per-app secret, shared with the push handler. Unsigned deliveries are rejected. |
| A drive-by commenter deploys | Write or admin permission is confirmed with GitHub for every command, cached for a short time, and fails closed on any API error. |
| A pull request from a fork runs code paths with the base repo's trust | Refused unless the owner turned on fork commands for that app. The head repository comes from the GitHub API. A deleted fork counts as a fork. |
| A bot loop, or Levelrail answering itself | Bot accounts, GitHub App comments and comments with Levelrail's reply marker are ignored. |
| Free text picks the target | The comment cannot name an app. Only the four fixed commands parse, and quoted text, indented text and code blocks never match. |
| A flood of comments | Per-user and per-pull-request limits held in bounded memory, and a bounded table of handled comment ids. |
| Replay of the same delivery | Comment ids are recorded once per app. |
| Bypassing a freeze, an approval or a policy | Freezes, running-deploy guards, protected environments and deploy policy are checked first. A frozen app, or one that needs approval, gets a reply saying why and nothing runs. There is no override from a comment. |
| Markdown, mentions or links injected through the reply | Echoed values are reduced to plain word characters, shown in code spans, and cut to a short length. |
| A stolen permission after a role change | Permission answers are cached for `APP_PR_COMMENT_PERMISSION_CACHE` only. Refusals are never cached. |
| An abused installation | `APP_PR_COMMENT_COMMANDS_DISABLED` turns the feature off for the whole control plane without a deploy. |

Secrets are never logged or echoed. Logs carry the app name, pull request number and GitHub login.

## What it does not do

- It does not deploy the pull request's code. `redeploy` deploys the tracked branch.
- It does not approve a deploy or override a freeze. Use the dashboard for those.
- It does not run for GitLab, Bitbucket or Gitea.
- It does not edit your repository's webhooks.
- It does not accept a command from someone who only has read or triage access.
