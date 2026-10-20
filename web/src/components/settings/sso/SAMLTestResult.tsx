import { useTranslation } from 'react-i18next'
import {
  CheckCircleIcon,
  WarningCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import type { SAMLConnection } from '@/queries/sso'
import { connectionInput, useSaveSAMLConnection } from '@/queries/sso'
import type { SSOFix, SSOIssue, SSOTestResult } from '@/queries/ssoOidc'
import type { RoleResource } from '@/queries/roles'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'

const KNOWN_FAILURES = [
  'clock_skew',
  'audience_mismatch',
  'signature_invalid',
  'request_mismatch',
  'response_invalid',
  'saml_unavailable',
] as const

function applyFix(c: SAMLConnection, fix: SSOFix) {
  const input = connectionInput(c)
  if (fix.action === 'add_domain' && fix.value)
    input.allowed_domains = [...input.allowed_domains, fix.value]
  if (fix.action === 'enable_jit') input.jit_provisioning = true
  return input
}

function IssueRow({
  issue,
  connection,
}: {
  issue: SSOIssue
  connection: SAMLConnection
}) {
  const { t } = useTranslation('sso')
  const save = useSaveSAMLConnection()
  const { fix } = issue
  return (
    <li className="flex flex-wrap items-center gap-x-3 gap-y-2 py-2">
      <span className="min-w-0 flex-1 text-sm">
        {t(`saml.test.issue.${issue.code}`, {
          value: fix.value ?? '',
          attribute: connection.groups_attribute,
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
              { id: connection.id, input: applyFix(connection, fix) },
              {
                onSuccess: () =>
                  toast.add({ title: t('saml.test.fixed'), type: 'success' }),
              },
            )
          }
        >
          {t(`oidc.test.fix.${fix.action}`, { value: fix.value ?? '' })}
        </Button>
      ) : (
        <span className="text-xs text-muted-foreground">
          {t('saml.test.fixManual')}
        </span>
      )}
    </li>
  )
}

/** SAMLTestResult shows what a test sign-in received and what a real one would do. */
export function SAMLTestResult({
  result,
  connection,
  roles,
}: {
  result: SSOTestResult
  connection: SAMLConnection
  roles: RoleResource[]
}) {
  const { t } = useTranslation('sso')
  if (result.status === 'failed' || !result.evaluation) {
    const key =
      KNOWN_FAILURES.find((k) => k === result.failure_code) ??
      'response_invalid'
    return (
      <Alert variant="destructive">
        <WarningCircleIcon />
        <AlertTitle>{t(`saml.test.failure.${key}.title`)}</AlertTitle>
        <AlertDescription className="space-y-1">
          <p>
            {t(`saml.test.failure.${key}.step`, {
              entityId: connection.sp_entity_id,
              acs: connection.sp_acs_url,
            })}
          </p>
          <p className="text-xs">
            {t('saml.test.failureCode', { code: result.failure_code ?? '' })}
          </p>
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
          <CheckCircleIcon className="size-5 text-success" aria-hidden="true" />
        ) : (
          <WarningCircleIcon
            className="size-5 text-destructive"
            aria-hidden="true"
          />
        )}
        {ev.allowed ? t('saml.test.allowed') : t('saml.test.refused')}
      </div>
      <dl className="grid gap-x-4 gap-y-1 text-sm sm:grid-cols-[auto_1fr]">
        <dt className="text-muted-foreground">{t('saml.test.email')}</dt>
        <dd>{ev.email || t('saml.test.none')}</dd>
        <dt className="text-muted-foreground">{t('oidc.test.groups')}</dt>
        <dd>
          {ev.groups.length > 0 ? ev.groups.join(', ') : t('saml.test.none')}
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
            <IssueRow key={i.code} issue={i} connection={connection} />
          ))}
        </ul>
      ) : null}
      <p className="text-xs text-muted-foreground">
        {t('oidc.test.nothingCreated')}
      </p>
    </div>
  )
}
