import { useTranslation } from 'react-i18next'
import { CircleAlertGlassIcon, CircleCheckGlassIcon } from '@thesvg/ui-react'
import type {
  OAuthSSOPolicy,
  SSOFix,
  SSOIssue,
  SSOTestResult,
} from '@/queries/ssoOidc'
import { policyInput, useSaveOAuthSSOPolicy } from '@/queries/ssoOidc'
import type { RoleResource } from '@/queries/roles'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'

function applyFix(policy: OAuthSSOPolicy, fix: SSOFix) {
  const input = policyInput(policy)
  switch (fix.action) {
    case 'set_groups_claim':
      input.groups_claim = fix.value ?? input.groups_claim
      break
    case 'add_domain':
      if (fix.value)
        input.allowed_domains = [...input.allowed_domains, fix.value]
      break
    case 'enable_jit':
      input.jit_provisioning = true
      break
  }
  return input
}

function IssueRow({
  issue,
  policy,
  claim,
}: {
  issue: SSOIssue
  policy: OAuthSSOPolicy
  claim: string
}) {
  const { t } = useTranslation('sso')
  const save = useSaveOAuthSSOPolicy()
  const { fix } = issue
  return (
    <li className="flex flex-wrap items-center gap-x-3 gap-y-2 py-2">
      <span className="min-w-0 flex-1 text-sm">
        {t(`oidc.test.issue.${issue.code}`, {
          claim,
          value: fix.value ?? '',
        })}
      </span>
      {fix.kind === 'auto' && fix.action ? (
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={save.isPending}
          onClick={() =>
            save.mutate(
              { provider: policy.provider, input: applyFix(policy, fix) },
              {
                onSuccess: () =>
                  toast.add({ title: t('oidc.test.fixed'), type: 'success' }),
              },
            )
          }
        >
          {t(`oidc.test.fix.${fix.action}`, { value: fix.value ?? '' })}
        </Button>
      ) : (
        <span className="text-xs text-muted-foreground">
          {t('oidc.test.fixManual')}
        </span>
      )}
    </li>
  )
}

/** OIDCTestResult shows what a test login saw and what the sign-in would do. */
export function OIDCTestResult({
  result,
  policy,
  roles,
}: {
  result: SSOTestResult
  policy: OAuthSSOPolicy
  roles: RoleResource[]
}) {
  const { t } = useTranslation('sso')
  if (result.status === 'failed' || !result.evaluation) {
    return (
      <Alert variant="destructive">
        <CircleAlertGlassIcon className="text-tone-danger" size="1.2em" />
        <AlertTitle>{t('oidc.test.failedTitle')}</AlertTitle>
        <AlertDescription>
          {t('oidc.test.failedBody', { code: result.failure_code ?? '' })}
        </AlertDescription>
      </Alert>
    )
  }
  const ev = result.evaluation
  const role = roles.find((r) => r.id === ev.role_id)?.name ?? ev.role_id
  return (
    <div className="space-y-3 rounded-lg border border-border bg-card p-4">
      <div className="flex items-center gap-2 text-sm font-medium">
        {ev.allowed ? (
          <CircleCheckGlassIcon
            className="size-5.5 text-success"
            aria-hidden="true"
          />
        ) : (
          <CircleAlertGlassIcon
            className="size-5.5 text-destructive"
            aria-hidden="true"
          />
        )}
        {ev.allowed ? t('oidc.test.allowed') : t('oidc.test.refused')}
      </div>
      <dl className="grid gap-x-4 gap-y-1 text-sm sm:grid-cols-[auto_1fr]">
        <dt className="text-muted-foreground">{t('oidc.test.groups')}</dt>
        <dd>
          {ev.groups.length > 0
            ? ev.groups.join(', ')
            : t('oidc.test.noGroups')}
        </dd>
        <dt className="text-muted-foreground">{t('oidc.test.role')}</dt>
        <dd>
          {role} ({t(`oidc.test.source.${ev.role_source}`)})
        </dd>
        <dt className="text-muted-foreground">{t('oidc.test.account')}</dt>
        <dd>{t(`oidc.test.accountState.${ev.account_state}`)}</dd>
      </dl>
      {ev.issues.length > 0 ? (
        <ul className="divide-y divide-border">
          {ev.issues.map((i) => (
            <IssueRow
              key={i.code}
              issue={i}
              policy={policy}
              claim={ev.groups_claim}
            />
          ))}
        </ul>
      ) : null}
      <p className="text-xs text-muted-foreground">
        {t('oidc.test.nothingCreated')}
      </p>
    </div>
  )
}
