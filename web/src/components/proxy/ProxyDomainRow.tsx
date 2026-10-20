import { useTranslation } from 'react-i18next'
import { SpinnerIcon } from '@phosphor-icons/react/dist/ssr'
import { Badge, type badgeVariants } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import type { VariantProps } from 'class-variance-authority'
import {
  domainLiveState,
  type DomainLiveState,
} from '../../lib/proxyIntegration'
import {
  useVerifyProxyDomain,
  type ProxyDomain,
} from '../../queries/proxyIntegration'

type BadgeVariant = VariantProps<typeof badgeVariants>['variant']

const STATE_VARIANT: Record<DomainLiveState, BadgeVariant> = {
  todo: 'muted',
  handled: 'info',
  waiting: 'warning',
  live: 'success',
  error: 'destructive',
}

export function ProxyStateBadge({ row }: { row: ProxyDomain }) {
  const { t } = useTranslation('domains')
  const state = domainLiveState(row)
  return (
    <Badge variant={STATE_VARIANT[state]}>
      {t(`proxySetup.domains.state.${state}`)}
    </Badge>
  )
}

function formatCertDate(iso: string): string {
  return new Date(iso).toLocaleDateString(undefined, {
    day: 'numeric',
    month: 'short',
    year: 'numeric',
  })
}

export function ProxyCertificateLine({ row }: { row: ProxyDomain }) {
  const { t } = useTranslation('domains')
  if (domainLiveState(row) === 'todo') return null
  const cert = row.certificate
  const text = cert
    ? t(
        cert.valid
          ? 'proxySetup.domains.certificate.issued'
          : 'proxySetup.domains.certificate.expired',
        { issuer: cert.issuer, date: formatCertDate(cert.not_after) },
      )
    : t('proxySetup.domains.certificate.pending')
  return <p className="text-xs text-muted-foreground">{text}</p>
}

export function ProxyVerifyButton({ row }: { row: ProxyDomain }) {
  const { t } = useTranslation('domains')
  const verify = useVerifyProxyDomain()
  const domain = row.domain
  return (
    <Button
      type="button"
      size="sm"
      variant="outline"
      disabled={verify.isPending}
      onClick={() => {
        verify.mutate(domain, {
          onSuccess: (result) => {
            const live = domainLiveState(result) === 'live'
            toast.add({
              title: t(
                live
                  ? 'proxySetup.domains.verifyLive'
                  : 'proxySetup.domains.verifyNotLive',
                { domain },
              ),
              description: result.last_error || undefined,
              type: live ? 'success' : 'warning',
            })
          },
          onError: (error) => {
            toast.add({
              title: t('proxySetup.domains.verifyFailed', { domain }),
              description: error.message,
              type: 'error',
            })
          },
        })
      }}
    >
      {verify.isPending ? (
        <SpinnerIcon className="animate-spin" aria-hidden="true" />
      ) : null}
      {verify.isPending
        ? t('proxySetup.domains.verifying')
        : t('proxySetup.domains.verify')}
    </Button>
  )
}

export function ProxyDomainRow({ row }: { row: ProxyDomain }) {
  const { t } = useTranslation('domains')
  const checked = row.checked_at
    ? t('proxySetup.domains.checked', {
        time: new Date(row.checked_at).toLocaleTimeString(),
      })
    : t('proxySetup.domains.neverChecked')
  return (
    <li className="flex flex-wrap items-start justify-between gap-3 py-3">
      <div className="min-w-0 space-y-1">
        <div className="flex flex-wrap items-center gap-2">
          <span className="font-mono text-sm break-all text-foreground">
            {row.domain}
          </span>
          <ProxyStateBadge row={row} />
        </div>
        <p className="text-xs text-muted-foreground">
          {row.target === 'app'
            ? t('proxySetup.domains.target.app', { app: row.app })
            : t('proxySetup.domains.target.dashboard')}
        </p>
        <ProxyCertificateLine row={row} />
        {row.last_error ? (
          <p className="text-xs text-destructive">
            {t('proxySetup.domains.lastError', { error: row.last_error })}
          </p>
        ) : null}
        <p className="text-xs text-muted-foreground">{checked}</p>
      </div>
      <ProxyVerifyButton row={row} />
    </li>
  )
}
