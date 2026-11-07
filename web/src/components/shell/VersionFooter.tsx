import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ArrowUpGlassIcon } from '@thesvg/ui-react'
import { TONE, type Tone } from '@/components/kit/tone'
import { cn } from '@/lib/utils'
import type { ShellStatus } from '@/lib/shellStatus'
import { displayVersion } from '../../lib/updateBehind'
import { updatesQueryOptions } from '../../queries/updates'
import { useShellStatus } from './useShellStatus'

const STATUS_TONE: Record<ShellStatus, Tone> = {
  healthy: 'success',
  update: 'warning',
  degraded: 'warning',
  incident: 'danger',
  busy: 'info',
  unknown: 'neutral',
}

export function VersionFooterView({
  current,
  latest,
  updateAvailable,
  status,
}: {
  current: string | undefined
  latest: string | null | undefined
  updateAvailable: boolean
  status: ShellStatus
}) {
  const { t } = useTranslation()
  // The update is its own line; the dot stays a health signal.
  const health = status === 'update' ? 'healthy' : status
  const version = current ? displayVersion(current) : null
  const update =
    updateAvailable && latest
      ? t('shell.account.updateAvailable', { version: displayVersion(latest) })
      : null
  const healthLabel = t(`shell.logoStatus.${health}`)
  const name = [t('shell.account.controlPlane'), version]
    .filter(Boolean)
    .join(' ')
  const label = [name, healthLabel, update].filter(Boolean).join('. ')
  return (
    <Link
      to="/settings/updates"
      aria-label={label}
      title={label}
      className="flex h-7 cursor-pointer items-center gap-2 truncate rounded-md px-2.5 text-xs text-sidebar-foreground/80 transition-colors duration-150 ease-out hover:bg-state-accent/10 hover:text-sidebar-foreground active:bg-state-accent/15 motion-reduce:transition-none focus-visible:ring-2 focus-visible:ring-sidebar-ring focus-visible:outline-none group-data-[collapsible=icon]:justify-center"
    >
      <span
        data-testid="health-dot"
        data-status={health}
        aria-hidden="true"
        className={cn(
          'size-2 shrink-0 rounded-full',
          TONE[STATUS_TONE[health]].solid,
        )}
      />
      {version ? (
        <span className="font-mono group-data-[collapsible=icon]:hidden">
          {version}
        </span>
      ) : null}
      {update ? (
        <span
          data-testid="version-update"
          className="flex min-w-0 items-center gap-1 group-data-[collapsible=icon]:hidden"
        >
          <ArrowUpGlassIcon
            size="1.4em"
            aria-hidden="true"
            className={cn('size-3 shrink-0', TONE.warning.text)}
          />
          <span className="truncate underline-offset-2 hover:underline">
            {update}
          </span>
        </span>
      ) : null}
    </Link>
  )
}

export function VersionFooter() {
  const data = useQuery(updatesQueryOptions()).data
  const status = useShellStatus()
  return (
    <VersionFooterView
      current={data?.current_version}
      latest={data?.latest_version}
      updateAvailable={data?.update_available === true}
      status={status}
    />
  )
}
