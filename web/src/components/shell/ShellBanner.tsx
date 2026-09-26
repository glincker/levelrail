import { useSyncExternalStore } from 'react'
import { Link } from '@tanstack/react-router'
import {
  WarningCircleIcon,
  WifiSlashIcon,
  XIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import {
  dismissConnectionBanner,
  getConnectionSnapshot,
  retryNow,
  subscribeConnection,
} from '../../lib/connectionStore'
import { CleanUpDockerDialog } from '../CleanUpDockerDialog'
import { pickBanner, type PlatformIssue } from './platformIssues'
import { snoozeNow, useSnoozed } from './snoozeStore'
import { usePlatformIssues } from './usePlatformIssues'

function Bar({
  icon,
  children,
  action,
  onDismiss,
}: {
  icon: React.ReactNode
  children: React.ReactNode
  action?: React.ReactNode
  onDismiss: () => void
}) {
  return (
    <div
      role="alert"
      className="flex h-9 shrink-0 items-center gap-2 border-b border-destructive/20 bg-destructive/10 px-4 text-sm text-destructive"
    >
      <span aria-hidden="true" className="[&_svg]:size-4">
        {icon}
      </span>
      <span className="min-w-0 flex-1 truncate">{children}</span>
      {action}
      <Button
        type="button"
        variant="ghost"
        size="icon-sm"
        aria-label="Dismiss"
        title="Dismiss"
        onClick={onDismiss}
      >
        <XIcon />
      </Button>
    </div>
  )
}

export function ShellBanner() {
  const conn = useSyncExternalStore(
    subscribeConnection,
    getConnectionSnapshot,
    getConnectionSnapshot,
  )
  const issues = usePlatformIssues()
  const { isSnoozed } = useSnoozed()
  const kind = pickBanner({
    offline: conn.offline,
    connectionDismissed: conn.dismissed,
    issues,
    isSnoozed,
  })
  const find = (id: string): PlatformIssue | undefined =>
    issues.find((i) => i.id === id)

  if (kind === 'connection') {
    return (
      <Bar
        icon={<WifiSlashIcon />}
        onDismiss={dismissConnectionBanner}
        action={
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={conn.probing}
            onClick={() => void retryNow()}
          >
            Retry now
          </Button>
        }
      >
        Can&apos;t reach the control plane. Live updates are paused.
      </Bar>
    )
  }
  if (kind === 'docker') {
    return (
      <Bar
        icon={<WarningCircleIcon />}
        onDismiss={() => snoozeNow('banner:docker-down', '1h')}
        action={
          <Button size="sm" variant="outline" render={<Link to="/nodes" />}>
            View node
          </Button>
        }
      >
        Docker is unreachable. Deploys and restarts will fail.
      </Bar>
    )
  }
  if (kind === 'disk') {
    const detail = find('disk')?.detail ?? ''
    return (
      <Bar
        icon={<WarningCircleIcon />}
        onDismiss={() => snoozeNow('banner:disk', '1d')}
        action={<CleanUpDockerDialog />}
      >
        Disk almost full{detail ? `, ${detail}` : ''}.
      </Bar>
    )
  }
  return null
}
