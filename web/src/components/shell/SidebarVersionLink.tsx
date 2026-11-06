import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { updatesQueryOptions } from '../../queries/updates'
import { displayVersion } from '../../lib/updateBehind'
import { TONE, type Tone } from '@/components/kit/tone'
import { cn } from '@/lib/utils'
import type { ShellStatus } from '@/lib/shellStatus'
import { useShellStatus } from './useShellStatus'

const STATUS_TONE: Record<ShellStatus, Tone> = {
  healthy: 'success',
  update: 'warning',
  degraded: 'warning',
  incident: 'danger',
  busy: 'info',
  unknown: 'neutral',
}

export function SidebarVersionLinkView({
  current,
  latest,
  updateAvailable,
  status,
}: {
  current: string | undefined
  latest: string | null | undefined
  updateAvailable: boolean
  status?: ShellStatus
}) {
  const { t } = useTranslation()
  const label = current
    ? `${t('shell.account.controlPlane')} ${displayVersion(current)}`
    : t('shell.account.controlPlane')
  // The update is its own marker; the status dot stays a health signal.
  const health = status === 'update' ? 'healthy' : status
  const healthLabel = health ? t(`shell.logoStatus.${health}`) : null
  const hint =
    updateAvailable && latest
      ? t('shell.account.updateAvailable', { version: displayVersion(latest) })
      : null
  const full = [label, healthLabel, hint].filter(Boolean).join('. ')
  return (
    <Link
      to="/settings/updates"
      aria-label={full}
      title={full}
      className="flex h-6 cursor-pointer items-center gap-1.5 truncate rounded-md px-2 text-xs text-sidebar-foreground/60 transition-colors duration-150 ease-out hover:bg-sidebar-accent/70 hover:text-sidebar-foreground active:bg-sidebar-accent motion-reduce:transition-none focus-visible:ring-2 focus-visible:ring-sidebar-ring focus-visible:outline-none group-data-[collapsible=icon]:hidden"
    >
      {health ? (
        <span
          data-testid="health-dot"
          data-status={health}
          aria-hidden="true"
          className={cn(
            'size-1.5 shrink-0 rounded-full',
            TONE[STATUS_TONE[health]].solid,
          )}
        />
      ) : null}
      <span className="truncate">{label}</span>
      {hint ? (
        <span
          data-testid="update-dot"
          aria-hidden="true"
          className="size-1.5 shrink-0 rounded-full bg-tone-warning-solid"
        />
      ) : null}
    </Link>
  )
}

export function SidebarVersionLink() {
  const data = useQuery(updatesQueryOptions()).data
  const status = useShellStatus()
  return (
    <SidebarVersionLinkView
      current={data?.current_version}
      latest={data?.latest_version}
      updateAvailable={data?.update_available === true}
      status={status}
    />
  )
}
