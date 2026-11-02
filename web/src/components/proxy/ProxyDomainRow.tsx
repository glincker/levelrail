import { useTranslation } from 'react-i18next'
import { useCopyToClipboard } from '@/hooks/useCopyToClipboard'
import { LoaderGlassIcon } from '@thesvg/ui-react'
import { Badge, type badgeVariants } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import type { VariantProps } from 'class-variance-authority'
import {
  certWording,
  domainLiveState,
  tlsSummary,
  type TlsSummaryKind,
} from '../../lib/proxyIntegration'
import {
  useRetryProxyCert,
  useVerifyProxyDomain,
  type ProxyDomain,
} from '../../queries/proxyIntegration'

type BadgeVariant = VariantProps<typeof badgeVariants>['variant']

const SUMMARY_VARIANT: Record<TlsSummaryKind, BadgeVariant> = {
  unwired: 'muted',
  waiting: 'warning',
  ready: 'success',
  default: 'destructive',
}

function formatClock(iso: string): string {
  return new Date(iso).toLocaleTimeString(undefined, {
    hour: '2-digit',
    minute: '2-digit',
  })
}

// The single pill for a domain behind a proxy: certificate ready, waiting,
// served the proxy default, or route not wired.
export function ProxyStateBadge({ row }: { row: ProxyDomain }) {
  const { t } = useTranslation('domains')
  const kind = tlsSummary(row)
  const since = row.first_seen ? formatClock(row.first_seen) : ''
  return (
    <Badge variant={SUMMARY_VARIANT[kind]}>
      {kind === 'waiting' && since
        ? t('proxySetup.domains.summary.waitingSince', { time: since })
        : t(`proxySetup.domains.summary.${kind}`)}
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
    ? t(`proxySetup.domains.certificate.${certWording(row)}`, {
        issuer: cert.issuer,
        date: formatCertDate(cert.not_after),
      })
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
        <LoaderGlassIcon
          size="1.4em"
          className="animate-spin"
          aria-hidden="true"
        />
      ) : null}
      {verify.isPending
        ? t('proxySetup.domains.verifying')
        : t('proxySetup.domains.verify')}
    </Button>
  )
}

export function ProxyRetryButton({ row }: { row: ProxyDomain }) {
  const { t } = useTranslation('domains')
  const retry = useRetryProxyCert()
  return (
    <Button
      type="button"
      size="sm"
      disabled={retry.isPending}
      onClick={() => {
        retry.mutate(row.domain, {
          onError: (error) => {
            toast.add({
              title: t('proxySetup.domains.retryFailed', {
                domain: row.domain,
              }),
              description: error.message,
              type: 'error',
            })
          },
        })
      }}
    >
      {retry.isPending
        ? t('proxySetup.domains.retrying')
        : t('proxySetup.domains.retry')}
    </Button>
  )
}

export function ProxyDiagnosis({ row }: { row: ProxyDomain }) {
  const { t } = useTranslation('domains')
  const copy = useCopyToClipboard()
  if (!row.diagnosis && !row.hint) return null
  return (
    <div className="space-y-1 text-xs">
      {row.hint ? <p className="text-foreground">{row.hint}</p> : null}
      {row.evidence && row.evidence.length > 0 ? (
        <ul className="list-disc space-y-0.5 pl-4 break-words text-muted-foreground">
          {row.evidence.map((e) => (
            <li key={e}>{e}</li>
          ))}
        </ul>
      ) : null}
      {row.snippet ? (
        <div className="flex items-start gap-2">
          <code className="min-w-0 flex-1 rounded bg-muted px-2 py-1 font-mono break-all whitespace-pre-wrap">
            {row.snippet}
          </code>
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() => copy.copy(row.snippet ?? '')}
          >
            {copy.copied
              ? t('proxySetup.domains.copied')
              : t('proxySetup.domains.copy')}
          </Button>
        </div>
      ) : null}
    </div>
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
