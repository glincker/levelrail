---
description: Roles for multi-user access, per-user environment grants, the guest role, IAM policies on apps, databases and environments, and ready-made policy templates.
---

# Access control

Give each teammate the access they need and no more. A role bundles abilities, environment grants narrow a guest down to chosen environments, and IAM policies allow or deny access to individual apps, databases and environments.

<InlineToc default-open />

Managing stored roles and environment grants is behind the `access-roles` experimental flag: set `APP_EXPERIMENTAL=access-roles` on the control plane (see [Experimental features](experimental-features.md)). Listing roles, IAM policies and the environment rules below are always on.

## Built-in roles

Four roles exist on every instance. They cannot be edited or deleted.

| Role | Abilities | Visibility | Use for |
| --- | --- | --- | --- |
| `admin` | `root` | all | Full control |
| `operator` | `read`, `read:sensitive`, `write`, `deploy` | all | Deploy and configure |
| `viewer` | `read` | all | Read-only access |
| `guest` | `read` | granted | Read-only access to the environments you grant |

## Abilities

Every user and every API token carries abilities. A role is a named set of them.

| Ability | Allows |
| --- | --- |
| `read` | View apps, logs, metrics, deploy history |
| `read:sensitive` | View secrets and other private config |
| `write` | Change config, restart, manage domains and env vars |
| `write:sensitive` | Rotate secrets and update sensitive values |
| `deploy` | Trigger a deploy |
| `root` | Everything. Exclusive: it cannot be combined with other abilities |

See [Identity and access](identity-and-access.md) for how abilities, sessions and tokens work.

## Custom roles

Create a role with a name, the abilities it holds and a visibility:

```bash
levelrail-cli roles create "Platform lead" \
  --abilities read,read:sensitive,write,deploy \
  --description "Deploys and configures, never rotates secrets"
levelrail-cli roles list
levelrail-cli roles update "Platform lead" --abilities read,write,deploy
levelrail-cli roles delete "Platform lead"
```

A role can be given by name or id. In the dashboard, open Settings, Team, Roles.

Rules the server enforces:

- Built-in roles cannot be edited or deleted.
- A role that is assigned to users cannot be deleted. Reassign them first.
- Editing a custom role updates the abilities of everyone who holds it, in one step.
- The last user with `root` cannot lose it, whether by a role change, a role edit or a deletion.
- You cannot change your own role. Ask another admin.

## Assign a role to a user

```bash
levelrail-cli users create --email ops@example.com --password 'change-me-now' --role operator
levelrail-cli users role set user_abc operator
```

Invites accept a role too. Editing a user's abilities by hand removes their role label, because the abilities no longer match any role.

## Visibility and the guest role

A role's visibility is `all` (the default) or `granted`.

- With `all`, a user sees every app and database their abilities allow.
- With `granted`, a user sees only apps and databases in the environments granted to that user. A role with `granted` visibility may hold only the `read` ability.

Anything a guest cannot see answers **404**, not 403, so they cannot even tell it exists. Resources with no environment are hidden from guests. API tokens follow the visibility of the user who owns them. The apps, databases, deployments and approvals lists are filtered the same way.

### Environment grants

```bash
levelrail-cli users grants set user_abc --environment env_dev --environment env_test
levelrail-cli users grants get user_abc
levelrail-cli users grants set user_abc     # no --environment: clear all grants
```

`grants set` replaces the whole list. A user with no grants sees nothing. Grants only apply to users whose role has `granted` visibility. In the dashboard, the Users page shows an environment checklist when you change a user to such a role. See [Environments](environments.md) for the environments you can grant.

## IAM policies

A policy adds Allow or Deny rules on top of a user's or token's abilities:

```json
{
  "Statement": [
    { "Effect": "Allow", "Action": ["deploy"], "Resource": ["app:checkout"] },
    { "Effect": "Deny", "Action": ["deploy"], "Resource": ["environment-kind:production"] }
  ]
}
```

Evaluation order: an explicit Deny wins, then the user's own abilities, then a matching Allow.

Resource strings:

| Resource | Matches |
| --- | --- |
| `app:{name}` | One app |
| `database:{name}` | One database |
| `environment:{id}` | Every app and database tagged with that environment |
| `environment-kind:{kind}` | Every app and database in any environment of that kind: `dev`, `test`, `uat`, `production`, `preview` or `custom` |

A trailing `*` matches a prefix (`app:*`, `environment:*`, `environment-kind:*`). An app or database with no environment matches no environment rule.

```bash
levelrail-cli iam policies create --name no-prod-deploys \
  --document '{"Statement":[{"Effect":"Deny","Action":["deploy"],"Resource":["environment-kind:production"]}]}'
levelrail-cli iam policies attach pol_abc --principal-type user --principal-id user_xyz
levelrail-cli iam policies detach pol_abc --principal-type user --principal-id user_xyz
```

### Policy templates

Five ready-made policies. In the dashboard use Settings, IAM policies, Add from template: it shows the effect in plain language and the JSON before you create it.

| Template | Effect |
| --- | --- |
| `read-only` | Read access only |
| `guest-one-environment` | Read access to one environment (parameter `environment`, which must exist) |
| `deployer-nonprod` | Read, write and deploy on `dev`, `test`, `uat` and `preview`. Denies write, `write:sensitive` and deploy on `production` |
| `ai-operator-nonprod` | Same allow as `deployer-nonprod`. Denies every mutating ability on `production` and `root` everywhere |
| `production-approver` | Read and deploy on `production` |

```bash
levelrail-cli iam templates list
levelrail-cli iam templates apply guest-one-environment --param environment=env_dev --attach-user user_abc
```

`apply` accepts `--name`, `--attach-user` and `--attach-token`. A duplicate policy name answers 409.

## Audit

Every request above `read` is written to the audit log with who made it and which ability it needed:

```bash
levelrail-cli audit-log --client-kind cli --method POST
levelrail-cli audit-log --format csv --output-file audit-export.csv
```

## Build, test and review policies

Open **Settings, IAM policies** for the policy workspace. How the evaluator works is written down in [IAM design notes](iam-design.md).

- **Guided builder.** Pick Allow or Deny, choose abilities from a grouped list with a plain description and a risk hint, then choose resources by picker: everything, a project (expanded to its environments), an environment, an environment kind, one app, one database or a name pattern. Each choice shows how many apps and databases it matches today. The stored JSON is shown beside the rules and is only editable behind the **Edit JSON** switch. Errors point at the field path.
- **Templates with parameters.** Read only, guest in one environment, deployer for non-production, operator of one app, owner of one database and deployer for one project. You pick the app, database, environment or project instead of typing an ID.
- **Simulator.** Ask whether a user or token can do an ability on a resource. The answer comes from the same evaluator that guards requests and lists the deciding statement and every statement that matched. The same check runs from the CLI and the read-only MCP tools.
- **Who has access.** Start from a user or token: its abilities, attached policies and what it can do on every app and database. Attach or detach with a preview of the access that changes, in bulk for tokens.
- **Analyzer.** Static checks with a severity and a one line fix: allow everything, wildcard or root abilities, sensitive abilities on every resource, a Deny that can never match, unattached policies, tokens holding root, attachments to a missing user or token, shadowed or overlapping statements and keys the evaluator ignores.
- **Safe changes.** Every saved document is kept as a version with a rule diff. A change that would leave no operational root principal is refused.

Conditions such as source IP, time window and MFA are not part of the evaluator yet, so the builder does not offer them. Scope by environment with resources instead.

```bash
levelrail-cli iam simulate --principal token:TOKEN_ID --action deploy --resource app:web
levelrail-cli iam effective --principal user:USER_ID
levelrail-cli iam analyze --json
```

## API

| Method | Path | Ability |
| --- | --- | --- |
| `GET` | `/api/v1/roles` | `read` |
| `POST` | `/api/v1/roles` | `root` |
| `PUT` | `/api/v1/roles/{id}` | `root` |
| `DELETE` | `/api/v1/roles/{id}` | `root` |
| `PUT` | `/api/v1/users/{id}/role` | `root` |
| `GET` | `/api/v1/users/{id}/environment-grants` | `root` |
| `PUT` | `/api/v1/users/{id}/environment-grants` | `root` |
| `GET` | `/api/v1/iam/policy-templates` | `read` |
| `POST` | `/api/v1/iam/policy-templates/{id}/apply` | `root` |
| `POST` | `/api/v1/iam/policy-templates/{id}/render` | `read` |
| `GET` | `/api/v1/iam/catalog` | `read` |
| `GET` | `/api/v1/iam/resources` | `read` |
| `POST` | `/api/v1/iam/resources/match` | `read` |
| `GET` | `/api/v1/iam/principals` | `read` |
| `GET` | `/api/v1/iam/principals/{type}/{id}/effective` | `read` |
| `GET` | `/api/v1/iam/simulate` | `read` |
| `GET` | `/api/v1/iam/analyze` | `read` |
| `POST` | `/api/v1/iam/policies/validate` | `read` |
| `POST` | `/api/v1/iam/preview` | `read` |
| `GET` | `/api/v1/iam/policies/{id}/versions` | `read` |

Role bodies are `{"name", "description", "abilities", "visibility"}`. Grants are `{"environment_ids": []}`.

## Next steps

<CardGroup :cols="2">
<Card title="Environments" href="/environments">

Instance-wide environments and protected moves.

</Card>
<Card title="AI control" href="/ai-control">

Decide what agents may do.

</Card>
</CardGroup>
