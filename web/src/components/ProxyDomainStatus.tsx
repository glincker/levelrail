import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { domainLiveState, findProxyDomain } from '../lib/proxyIntegration'
import {
  proxyIntegrationQueryOptions,
  useSetupProxyIntegration,
} from '../queries/proxyIntegration'
import {
  ProxyCertificateLine,
  ProxyStateBadge,
  ProxyVerifyButton,
} from './proxy/ProxyDomainRow'
import { ProxySetupConfirm } from './proxy/ProxySetupConfirm'

// Per-domain proxy status for the app Domains tab. Renders nothing when
// no proxy is detected or this server has no proxy integration.
export function ProxyDomainStatus({ domain }: { domain: string }) {
  const { t } = useTranslation('domains')
  const { data } = useQuery(proxyIntegrationQueryOptions())
  const setup = useSetupProxyIntegration()
  const [confirming, setConfirming] = useState(false)
  if (!data || data.detected.kind === 'none') return null
  const row = findProxyDomain(data, domain)
  if (!row) return null
  const state = domainLiveState(row)

  return (
    <div className="space-y-1.5 rounded-md border border-border bg-muted/30 p-3 text-xs">
      <div className="flex flex-wrap items-center gap-2">
        <ProxyStateBadge row={row} />
        {state === 'todo' ? (
          <Button
            type="button"
            size="sm"
            disabled={!data.detected.complete || setup.isPending}
            onClick={() => setConfirming(true)}
          >
            {t('proxySetup.domains.makeLive')}
          </Button>
        ) : (
          <ProxyVerifyButton row={row} />
        )}
      </div>
      <ProxyCertificateLine row={row} />
      {row.last_error ? (
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
      <Badge variant="info">{t('proxySetup.handledByProxy')}</Badge>
      {row?.certificate ? <ProxyCertificateLine row={row} /> : null}
    </>
  )
}
