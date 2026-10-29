import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CircleCheckGlassIcon,
  GlobeGlassIcon,
  LockGlassIcon,
  LockOpenGlassIcon,
  NetworkGlassIcon,
  PlugGlassIcon,
  ServerGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import type { ProbeTarget } from '../../queries/appExposure'
import { reasonKey, type ReachScope, type ReachSecurity } from './reachModel'

const SCOPE_ICON = {
  internet: GlobeGlassIcon,
  network: NetworkGlassIcon,
  server: ServerGlassIcon,
  apps: PlugGlassIcon,
} as const

export function ScopeChip({ scope }: { scope: ReachScope }) {
  const { t } = useTranslation('appAccess')
  const Icon = SCOPE_ICON[scope]
  return (
    <Badge variant="muted">
      <Icon size="1.4em" aria-hidden="true" />
      {t(`scope.${scope}`)}
    </Badge>
  )
}

export function SecurityChip({ security }: { security: ReachSecurity }) {
  const { t } = useTranslation('appAccess')
  if (security === 'none') return null
  const Icon =
    security === 'secure'
      ? LockGlassIcon
      : security === 'selfSigned'
        ? LockOpenGlassIcon
        : TriangleAlertGlassIcon
  const variant =
    security === 'secure'
      ? 'success'
      : security === 'selfSigned'
        ? 'warning'
        : 'outline'
  return (
    <Badge variant={variant} className="whitespace-normal text-left">
      <Icon size="1.4em" aria-hidden="true" />
      {t(`security.${security}`)}
    </Badge>
  )
}

/** One plain sentence for a probe, with the raw error behind a toggle. */
export function ProbeNote({ probe }: { probe: ProbeTarget }) {
  const { t } = useTranslation('appAccess')
  const [open, setOpen] = useState(false)
  if (probe.ok) {
    return (
      <p className="flex items-center gap-1 text-xs text-tone-success">
        <CircleCheckGlassIcon
          className="size-5 text-tone-success"
          aria-hidden="true"
        />
        {t('test.ok', { ms: probe.latency_ms ?? 0 })}
      </p>
    )
  }
  return (
    <div className="text-xs text-tone-warning">
      <p className="flex flex-wrap items-center gap-x-2">
        <span className="flex items-center gap-1">
          <TriangleAlertGlassIcon
            className="size-5 text-tone-warning"
            aria-hidden="true"
          />
          {t(`test.reason.${reasonKey(probe)}`)}
        </span>
        {probe.detail ? (
          <button
            type="button"
            aria-expanded={open}
            className="underline underline-offset-2"
            onClick={() => setOpen((v) => !v)}
          >
            {open ? t('actions.hideDetails') : t('actions.showDetails')}
          </button>
        ) : null}
      </p>
      {open && probe.detail ? (
        <code className="mt-1 block break-all rounded-md bg-muted px-2 py-1 font-mono text-muted-foreground">
          {probe.detail}
        </code>
      ) : null}
    </div>
  )
}
