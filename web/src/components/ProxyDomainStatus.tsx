import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { findProxyDomain, tlsSummary } from '../lib/proxyIntegration'
import {
  proxyIntegrationQueryOptions,
  useSetupProxyIntegration,
  type ProxyDomain,
} from '../queries/proxyIntegration'
import {
  ProxyCertificateLine,
  ProxyDiagnosis,
  ProxyRetryButton,
  ProxyStateBadge,
  ProxyVerifyButton,
} from './proxy/ProxyDomainRow'
import { useAutoRecheck } from '../hooks/useAutoRecheck'
import { ProxySetupConfirm } from './proxy/ProxySetupConfirm'

// Per-domain proxy status for the app Domains tab. Renders nothing when
// no proxy is detected or this server has no proxy integration. The pill is
// shown once in the TLS row; this block adds the one sentence and one action.
export function ProxyDomainStatus({ domain }: { domain: string }) {
  const { t } = useTranslation('domains')
  const { data } = useQuery(proxyIntegrationQueryOptions())
  const setup = useSetupProxyIntegration()
  const [confirming, setConfirming] = useState(false)
  const row = data ? findProxyDomain(data, domain) : undefined
  if (!data || data.detected.kind === 'none' || !row) return null

  return (
    <div className="space-y-1.5 rounded-md border border-border bg-muted/30 p-3 text-xs">
      <ProxyDomainAction
        row={row}
        makeLiveDisabled={!data.detected.complete || setup.isPending}
        onMakeLive={() => setConfirming(true)}
      />
      <ProxyDiagnosis row={row} />
      {!row.hint && row.last_error ? (
        <p className="text-destructive">
          {t('proxySetup.domains.lastError', { error: row.last_error })}
        </p>
      ) : null}
      <ProxySetupConfirm
        open={confirming}
        onOpenChange={setConfirming}
        directory={data.detected.dynamic_dir}
        domains={[domain]}
        pending={setup.isPending}
        onConfirm={() => {
          setup.mutate(
            {
              confirm: true,
              dynamic_dir: data.detected.dynamic_dir || undefined,
            },
            {
              onSuccess: () => {
                setConfirming(false)
                toast.add({ title: t('proxySetup.setupDone'), type: 'success' })
              },
              onError: (error) => {
                toast.add({
                  title: t('proxySetup.setupFailed'),
                  description: error.message,
                  type: 'error',
                })
              },
            },
          )
        }}
      />
    </div>
  )
}

// One primary action per state; auto rechecks run while a certificate is due.
function ProxyDomainAction({
  row,
  makeLiveDisabled,
  onMakeLive,
}: {
  row: ProxyDomain
  makeLiveDisabled: boolean
  onMakeLive: () => void
}) {
  const { t } = useTranslation('domains')
  useAutoRecheck(row)
  const kind = tlsSummary(row)
  if (kind === 'unwired') {
    return (
      <Button
        type="button"
        size="sm"
        disabled={makeLiveDisabled}
        onClick={onMakeLive}
      >
        {t('proxySetup.domains.makeLive')}
      </Button>
    )
  }
  if (kind === 'default') {
    return (
      <div className="flex flex-wrap items-center gap-2">
        <ProxyRetryButton row={row} />
        <ProxyVerifyButton row={row} />
      </div>
    )
  }
  return <ProxyVerifyButton row={row} />
}

// Replaces the endless "Provisioning" badge when a proxy terminates TLS.
export function ProxyHandledTlsBadge({
  domain,
  fallback,
}: {
  domain: string
  fallback: React.ReactNode
}) {
  const { t } = useTranslation('domains')
  const { data } = useQuery(proxyIntegrationQueryOptions())
  if (!data?.ingress.tls_terminated_upstream) return <>{fallback}</>
  const row = findProxyDomain(data, domain)
  return (
    <>
      {row ? (
        <ProxyStateBadge row={row} />
      ) : (
        <Badge variant="info">{t('proxySetup.handledByProxy')}</Badge>
      )}
      {row?.certificate ? <ProxyCertificateLine row={row} /> : null}
    </>
  )
}
