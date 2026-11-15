import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { BrandIcon } from '@/components/BrandIcon'
import { TONE } from '@/components/kit/tone'
import { brandForProvider } from '../../lib/deliveryBrand'
import { latestOfKind, postStatusFailing } from '../../lib/deliveryTimeline'
import { formatRelativeTime } from '../../lib/relativeTime'
import type { Connection, ConnectionHealth } from '../../types/connections'
import type { AppBinding, TimelineItem } from '../../types/appDelivery'
import { statusKey } from '../../lib/deliveryStatus'
import { StatusIcon } from './deliveryStatus'

const HEALTH_VARIANT: Record<
  ConnectionHealth,
  'success' | 'warning' | 'destructive' | 'muted'
> = {
  ok: 'success',
  degraded: 'warning',
  needs_reconnect: 'destructive',
  expired: 'destructive',
  unknown: 'muted',
}

function bindingFor(bindings: readonly AppBinding[], role: string) {
  return bindings.find((b) => b.role === role)
}

function Card({
  label,
  children,
}: {
  label: string
  children: React.ReactNode
}) {
  return (
    <div className="min-w-56 flex-1 rounded-lg border border-border bg-card px-3 py-2">
      <p className="text-xs text-muted-foreground">{label}</p>
      <div className="mt-1 flex flex-wrap items-center gap-2 text-sm">
        {children}
      </div>
    </div>
  )
}

function ConnectionCell({
  binding,
  connections,
}: {
  binding: AppBinding | undefined
  connections: readonly Connection[]
}) {
  const { t } = useTranslation('delivery')
  if (!binding) {
    return <span className="text-muted-foreground">{t('header.none')}</span>
  }
  const conn = connections.find((c) => c.ref === binding.ref)
  const brand = conn ? brandForProvider(conn.provider) : undefined
  const health = conn?.health ?? 'unknown'
  return (
    <>
      {brand ? <BrandIcon name={brand} className="size-4" /> : null}
      <span className="min-w-0 font-medium break-all">
        {conn?.label ?? binding.ref}
      </span>
      <Badge variant={HEALTH_VARIANT[health]}>{t(`health.${health}`)}</Badge>
    </>
  )
}

export function DeliveryHeader({
  items,
  bindings,
  connections,
}: {
  items: readonly TimelineItem[]
  bindings: readonly AppBinding[]
  connections: readonly Connection[]
}) {
  const { t } = useTranslation('delivery')
  const lastDeploy = latestOfKind(items, 'deploy')
  const failing = postStatusFailing(items)
  return (
    <section aria-label={t('header.label')} className="space-y-2">
      <div className="flex flex-wrap gap-2">
        <Card label={t('header.source')}>
          <ConnectionCell
            binding={bindingFor(bindings, 'source')}
            connections={connections}
          />
        </Card>
        <Card label={t('header.reporting')}>
          <ConnectionCell
            binding={bindingFor(bindings, 'deploy_report')}
            connections={connections}
          />
        </Card>
        <Card label={t('header.lastDeploy')}>
          {lastDeploy ? (
            <>
              <StatusIcon status={lastDeploy.status} />
              <span className="font-medium">
                {t(`status.${statusKey(lastDeploy.status)}`)}
              </span>
              <span className="text-xs text-muted-foreground">
                {formatRelativeTime(lastDeploy.at)}
              </span>
            </>
          ) : (
            <span className="text-muted-foreground">{t('header.none')}</span>
          )}
        </Card>
      </div>
      {failing ? (
        <p
          role="status"
          className={`flex items-center gap-2 rounded-md border px-3 py-2 text-sm ${TONE.danger.soft} ${TONE.danger.border} ${TONE.danger.text}`}
        >
          <TriangleAlertGlassIcon aria-hidden="true" className="size-4" />
          <span>{t('header.postFailing')}</span>
          <Link
            to="/settings/integrations"
            className="ml-auto font-medium underline underline-offset-2"
          >
            {t('row.openIntegrations')}
          </Link>
        </p>
      ) : null}
    </section>
  )
}
