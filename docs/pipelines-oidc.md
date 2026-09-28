---
description: Mint short-lived OIDC tokens for pipeline jobs, and wire AWS, GCP, or Vault to trust them without a long-lived credential.
---

# Pipelines: OIDC federation

A pipeline job normally reaches a cloud provider with a long-lived credential stored as a secret. OIDC federation replaces that with a short-lived, signed token the control plane mints per job run, verified by the provider against a published public key. Nothing long-lived ever sits in a secret or a job's environment.

## Enabling it

Set `APP_OIDC_ISSUER_URL` on the control plane to a real, reachable HTTPS URL, typically the same URL the dashboard is served on:

```
APP_OIDC_ISSUER_URL=https://cp.example.com
```

Without this, `oidc:` on a job fails with a clear error rather than minting a token nothing can verify: the issuer URL is also where a provider fetches the JWKS document to check the signature, so an unreachable or wrong URL breaks federation silently otherwise.

Check whether it's configured:

```
levelrail-cli pipelines oidc
```

Or in the dashboard: the **Pipelines** page shows a card with the JWKS URL once configured.

## Job configuration

```yaml
jobs:
  deploy:
    image: amazon/aws-cli
    oidc:
      audience: sts.amazonaws.com
    steps:
      - run: |
          aws sts assume-role-with-web-identity \
            --role-arn "$AWS_ROLE_ARN" \
            --web-identity-token "$PIPELINE_OIDC_TOKEN" \
            --role-session-name pipeline
```

`audience` is required and becomes the token's `aud` claim, the value the provider's trust policy checks. The token arrives as the `PIPELINE_OIDC_TOKEN` env var, masked in logs the same way a secret is, valid for 10 minutes (`APP_OIDC_TOKEN_TTL` overrides this).

A job that needs more than one audience (for example, both AWS and Vault in the same job) opts in once per audience by running separate jobs, or repeats the request-a-token pattern GitHub Actions uses if that becomes a real need; today, Levelrail controls the whole job lifecycle, so it injects the token directly as an env var rather than a request-URL indirection.

## Claims

```json
{
  "iss": "https://cp.example.com",
  "sub": "repo:web:ref:refs/heads/main:job:deploy",
  "aud": "sts.amazonaws.com",
  "iat": 1735689600,
  "nbf": 1735689600,
  "exp": 1735690200,
  "repo": "web",
  "ref": "refs/heads/main",
  "pipeline_id": "pl_abc123"
}
```

`sub` is `repo:<app>:ref:<git ref>:job:<job key>`. Match on `sub`, `repo`, or `ref` in a trust policy depending on how tightly scoped the role should be: a wildcard on `ref` trusts every branch, `refs/heads/main` exactly trusts only the main branch.

The signing key is ES256 (ECDSA P-256), generated once and persisted encrypted at rest, separate from the app secrets it never touches.

## JWKS

`GET /.well-known/jwks.json` on the control plane serves the public key, unauthenticated (this is how every OIDC verifier discovers it) and rate-limited per IP. Point AWS, GCP, or Vault's OIDC configuration at the issuer URL above; each fetches this path itself.

## Wiring to AWS IAM

1. IAM > Identity providers > Add provider > OpenID Connect.
   - Provider URL: the issuer URL (`APP_OIDC_ISSUER_URL`).
   - Audience: the same value the job's `oidc.audience` uses (`sts.amazonaws.com` is the AWS convention, but any string both sides agree on works, since this isn't a fixed AWS credential exchange, it's a `sts:AssumeRoleWithWebIdentity` call your job itself makes).
2. Create a role with a trust policy:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": { "Federated": "arn:aws:iam::<account-id>:oidc-provider/cp.example.com" },
      "Action": "sts:AssumeRoleWithWebIdentity",
      "Condition": {
        "StringEquals": { "cp.example.com:aud": "sts.amazonaws.com" },
        "StringLike": { "cp.example.com:sub": "repo:web:ref:refs/heads/main:job:*" }
      }
    }
  ]
}
```

3. Set `AWS_ROLE_ARN` as a job env var (or a secret) to the role's ARN, and call `aws sts assume-role-with-web-identity` as in the example above.

## Wiring to GCP workload identity federation

1. Create a workload identity pool and an OIDC provider inside it, issuer URI set to the issuer URL above, and an attribute mapping such as `google.subject=assertion.sub`.
2. Grant the target service account `roles/iam.workloadIdentityUser` scoped to the pool, with an attribute condition matching `sub`, `repo`, or `ref`.
3. Exchange the token for a Google access token via the STS `token` endpoint (`https://sts.googleapis.com/v1/token`) with `subject_token` set to `$PIPELINE_OIDC_TOKEN`, or use `gcloud auth login --cred-file` pointed at a generated credential config that references the token file path if the job writes it to disk first.

## Wiring to Vault

1. Enable the JWT auth method and configure it with `oidc_discovery_url` set to the issuer URL (Vault fetches the JWKS from there automatically).
2. Create a role with `bound_audiences` matching `oidc.audience`, and `bound_claims` matching `sub`, `repo`, or `ref` as needed.
3. In the job, `vault write auth/jwt/login role=<role> jwt="$PIPELINE_OIDC_TOKEN"`.

## What this does not cover

- No support yet for a job requesting more than one audience from a single `oidc:` block.
- No UI to browse past-issued tokens or their claims; nothing is persisted beyond the signing key itself, by design, since a token is meant to be short-lived and never logged.
- Key rotation is not yet exposed as an operator action; the signing key is generated once and reused. Rotating it (removing the persisted key so a new one generates) invalidates the JWKS a provider might have cached, which needs a re-fetch on their end.
