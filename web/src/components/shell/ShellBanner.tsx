import { useSyncExternalStore } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useRouterState } from '@tanstack/react-router'
import {
  CircleAlertGlassIcon,
  WifiOffGlassIcon,
  XGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import {
  dismissConnectionBanner,
  getConnectionSnapshot,
  retryNow,
  subscribeConnection,
} from '../../lib/connectionStore'
import { DockerDegradedBanner } from './DockerDegradedBanner'
import { useDockerDegraded } from './useDockerDegraded'
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
  const { t } = useTranslation('common', { useSuspense: false })
  return (
    <div
      role="alert"
      className="flex min-h-9 shrink-0 items-center gap-2 border-b border-destructive/20 bg-destructive/10 px-4 py-1 text-sm text-destructive"
    >
      <span aria-hidden="true" className="[&_svg]:size-5">
        {icon}
      </span>
      <span className="min-w-0 flex-1 break-words">{children}</span>
      {action}
      <Button
        type="button"
        variant="ghost"
        size="icon-sm"
        aria-label={t('shell.banner.dismiss')}
        title={t('shell.banner.dismiss')}
        onClick={onDismiss}
      >
        <XGlassIcon size="1.4em" />
      </Button>
    </div>
  )
}

export function ShellBanner() {
  const { t } = useTranslation('common', { useSuspense: false })
  const conn = useSyncExternalStore(
    subscribeConnection,
    getConnectionSnapshot,
    getConnectionSnapshot,
  )
  const issues = usePlatformIssues()
  const degraded = useDockerDegraded()
  const onDashboard = useRouterState({
    select: (s) => s.location.pathname === '/',
  })
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
        icon={<WifiOffGlassIcon size="1.4em" />}
        onDismiss={dismissConnectionBanner}
        action={
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={conn.probing}
            onClick={() => void retryNow()}
          >
            {t('shell.banner.retryNow')}
          </Button>
        }
      >
        {t('shell.banner.offline')}
      </Bar>
    )
  }
  if (degraded.active) {
    return <DockerDegradedBanner state={degraded} />
  }
  if (kind === 'docker') {
    return (
      <Bar
        icon={
          <CircleAlertGlassIcon className="text-tone-danger" size="1.4em" />
        }
        onDismiss={() => snoozeNow('banner:docker-down', '1h')}
        action={
          <Button size="sm" variant="outline" render={<Link to="/nodes" />}>
            {t('shell.banner.viewNode')}
          </Button>
        }
      >
        {t('shell.banner.dockerDown')}
      </Bar>
    )
  }
  // The dashboard renders its own disk Banner.
  if (kind === 'disk' && !onDashboard) {
    const detail = find('disk')?.detail ?? ''
    return (
      <Bar
        icon={
          <CircleAlertGlassIcon className="text-tone-danger" size="1.4em" />
        }
        onDismiss={() => snoozeNow('banner:disk', '1d')}
        action={<CleanUpDockerDialog />}
      >
        {detail
          ? t('shell.banner.diskFullDetail', { detail })
          : t('shell.banner.diskFull')}
      </Bar>
    )
  }
  return null
}
