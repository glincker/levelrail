import { RotateCwGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { useNowTick } from '../../hooks/useNowTick'
import type {
  LogStreamConnectionState,
  LogStreamTransport,
} from '../../hooks/useLogStream'
import { formatDurationMs } from '../../lib/deployDuration'
import { useDeployDetailT } from './useDeployDetailT'

/** DeployLiveStatus tells the truth about the log stream: last event, retry timer, fallback. */
export function DeployLiveStatus({
  transport,
  connectionState,
  lastEventAt,
  retryAt,
  onRetry,
}: {
  transport: LogStreamTransport
  connectionState: LogStreamConnectionState
  lastEventAt: number | null
  retryAt: number | null
  onRetry: () => void
}) {
  const { t } = useDeployDetailT()
  const ticking = transport !== 'closed'
  const now = useNowTick(ticking)

  if (transport === 'closed') {
    return <Badge variant="muted">{t('live.closed')}</Badge>
  }
  if (transport === 'polling') {
    return (
      <div className="flex flex-wrap items-center gap-2">
        <Badge variant="warning">{t('live.polling')}</Badge>
        <Button variant="outline" size="sm" onClick={onRetry}>
          <RotateCwGlassIcon className="size-5" data-icon="inline-start" />
          {t('live.retryNow')}
        </Button>
      </div>
    )
  }

  const lastEvent =
    lastEventAt === null
      ? t('live.noEvents')
      : t('live.lastEvent', {
          ago: formatDurationMs(Math.max(0, now - lastEventAt)) ?? '0s',
        })

  if (connectionState === 'error') {
    const seconds =
      retryAt === null ? null : Math.max(0, Math.ceil((retryAt - now) / 1000))
    return (
      <div className="flex flex-wrap items-center gap-2">
        <Badge variant="warning">
          {t('live.reconnecting')}
          {seconds === null ? '' : `, ${t('live.reconnectIn', { seconds })}`}
          {`, ${lastEvent}`}
        </Badge>
        <Button variant="outline" size="sm" onClick={onRetry}>
          <RotateCwGlassIcon className="size-5" data-icon="inline-start" />
          {t('live.retryNow')}
        </Button>
      </div>
    )
  }

  return (
    <Badge variant={connectionState === 'open' ? 'success' : 'muted'}>
      {connectionState === 'open' ? t('live.live') : t('live.connecting')}
      {`, ${lastEvent}`}
    </Badge>
  )
}
