import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { BrandIcon } from '@/components/BrandIcon'
import { TONE } from '@/components/kit/tone'
import { brandForProvider } from '../../lib/deliveryBrand'
import { latestOfKind, postStatusFailing } from '../../lib/deliveryTimeline'
import { formatRelativeTime } from '../../lib/relativeTime'
import type { Connection, ConnectionHealth } from '../../types/connections'
import type { ResolvedBinding, TimelineItem } from '../../types/appDelivery'
import { statusKey } from '../../lib/deliveryStatus'
import { StatusIcon } from './deliveryStatus'
import { BindConnectionDialog } from './BindConnectionDialog'
import { UnbindConnectionDialog } from './UnbindConnectionDialog'

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

const STRIP_ROLES = ['source', 'deploy_report']

// The effective row for a role: the app's own, else the org default it inherits.
function effectiveFor(bindings: readonly ResolvedBinding[], role: string) {
  return bindings.find((b) => b.role === role && !b.overridden)
}

function overriddenFor(bindings: readonly ResolvedBinding[], role: string) {
  return bindings.find((b) => b.role === role && b.overridden)
}

function isOwn(b: ResolvedBinding | undefined): b is ResolvedBinding {
  return b !== undefined && b.id !== undefined && !b.inherited_from
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
  overridden,
  connections,
}: {
  binding: ResolvedBinding | undefined
  overridden: ResolvedBinding | undefined
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
      {binding.inherited_from ? (
        <Badge variant="info">
          {t('header.inherited', { org: binding.inherited_from })}
        </Badge>
      ) : null}
      {overridden ? (
        <Badge variant="outline">
          {t('header.overrides', { org: overridden.inherited_from })}
        </Badge>
      ) : null}
    </>
  )
}

interface BindState {
  role: string
}

function RoleActions({
  binding,
  onBind,
  onUnbind,
}: {
  binding: ResolvedBinding | undefined
  onBind: () => void
  onUnbind: () => void
}) {
  const { t } = useTranslation('delivery')
  return (
    <span className="ml-auto flex gap-1">
      <Button variant="outline" size="sm" onClick={onBind}>
        {binding ? t('header.change') : t('header.bind')}
      </Button>
      {isOwn(binding) ? (
        <Button variant="outline" size="sm" onClick={onUnbind}>
          {t('header.unbind')}
        </Button>
      ) : null}
    </span>
  )
}

export function DeliveryHeader({
  app,
  items,
  bindings,
  connections,
}: {
  app: string
  items: readonly TimelineItem[]
  bindings: readonly ResolvedBinding[]
  connections: readonly Connection[]
}) {
  const { t } = useTranslation('delivery')
  const [bindState, setBindState] = useState<BindState | null>(null)
  const [unbindState, setUnbindState] = useState<ResolvedBinding | null>(null)
  const lastDeploy = latestOfKind(items, 'deploy')
  const failing = postStatusFailing(items)
  const others = bindings.filter(
    (b) => !STRIP_ROLES.includes(b.role) && !b.overridden,
  )
  const unbindConn = connections.find((c) => c.ref === unbindState?.ref)
  return (
    <section aria-label={t('header.label')} className="space-y-2">
      <div className="flex flex-wrap gap-2">
        {[
          { role: 'source', label: t('header.source') },
          { role: 'deploy_report', label: t('header.reporting') },
        ].map(({ role, label }) => {
          const binding = effectiveFor(bindings, role)
          return (
            <Card key={role} label={label}>
              <ConnectionCell
                binding={binding}
                overridden={overriddenFor(bindings, role)}
                connections={connections}
              />
              <RoleActions
                binding={binding}
                onBind={() => setBindState({ role })}
                onUnbind={() => setUnbindState(binding ?? null)}
              />
            </Card>
          )
        })}
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
      {others.length > 0 ? (
        <ul
          aria-label={t('header.other')}
          className="flex flex-wrap gap-2 text-sm"
        >
          {others.map((b) => {
            const conn = connections.find((c) => c.ref === b.ref)
            return (
              <li
                key={`${b.role}-${b.ref}`}
                className="flex items-center gap-2 rounded-lg border border-border bg-card px-3 py-1.5"
              >
                <span className="text-xs text-muted-foreground">
                  {t(`bind.roles.${b.role}`, { defaultValue: b.role })}
                </span>
                <span className="font-medium break-all">
                  {conn?.label ?? b.ref}
                </span>
                {b.inherited_from ? (
                  <Badge variant="info">
                    {t('header.inherited', { org: b.inherited_from })}
                  </Badge>
                ) : (
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setUnbindState(b)}
                  >
                    {t('header.unbind')}
                  </Button>
                )}
              </li>
            )
          })}
        </ul>
      ) : null}
      <div>
        <Button
          variant="outline"
          size="sm"
          onClick={() => setBindState({ role: '' })}
        >
          {t('header.bindOther')}
        </Button>
      </div>
      {failing ? (
        <p
          role="status"
          className={`flex items-center gap-2 rounded-md border px-3 py-2 text-sm ${TONE.danger.soft} ${TONE.danger.border} ${TONE.danger.text}`}
        >
          <TriangleAlertGlassIcon aria-hidden="true" className="size-4" />
          <span>{t('header.postFailing')}</span>
          <Link
            to="/integrations"
            className="ml-auto font-medium underline underline-offset-2"
          >
            {t('row.openIntegrations')}
          </Link>
        </p>
      ) : null}
      {bindState ? (
        <BindConnectionDialog
          open
          onOpenChange={(o) => {
            if (!o) setBindState(null)
          }}
          app={app}
          role={bindState.role}
          connections={connections}
        />
      ) : null}
      {unbindState?.id ? (
        <UnbindConnectionDialog
          open
          onOpenChange={(o) => {
            if (!o) setUnbindState(null)
          }}
          app={app}
          bindingId={unbindState.id}
          connectionRef={unbindState.ref}
          label={unbindConn?.label ?? unbindState.ref}
        />
      ) : null}
    </section>
  )
}
