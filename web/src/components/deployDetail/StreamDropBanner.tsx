import {
  ArrowClockwiseIcon,
  DownloadSimpleIcon,
  WifiSlashIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Button, buttonVariants } from '@/components/ui/button'
import { useNowTick } from '../../hooks/useNowTick'
import type {
  LogStreamConnectionState,
  LogStreamTransport,
} from '../../hooks/useLogStream'
import { useDeployDetailT } from './useDeployDetailT'

/** StreamDropBanner makes a dropped log stream impossible to miss and offers the escape hatches. */
export function StreamDropBanner({
  transport,
  connectionState,
  retryAt,
  onRetry,
  onSwitchToPolling,
  downloadUrl,
}: {
  transport: LogStreamTransport
  connectionState: LogStreamConnectionState
  retryAt: number | null
  onRetry: () => void
  onSwitchToPolling: () => void
  downloadUrl: string
}) {
  const { t } = useDeployDetailT()
  const dropped = transport === 'sse' && connectionState === 'error'
  const now = useNowTick(dropped)
  if (!dropped) {
    return null
  }
  const seconds =
    retryAt === null ? null : Math.max(0, Math.ceil((retryAt - now) / 1000))

  return (
    <div className="flex flex-wrap items-center gap-x-3 gap-y-2 rounded-lg border border-tone-warning-border bg-tone-warning-soft px-3 py-2 text-sm">
      <WifiSlashIcon className="size-4 shrink-0" aria-hidden="true" />
      <div className="min-w-0 flex-1">
        {/* Only the static text is live; the per-second countdown would be re-announced every tick. */}
        <div role="status">
          <p className="font-medium text-foreground">{t('stream.title')}</p>
          <p className="text-muted-foreground">{t('stream.bodyNoRetry')}</p>
        </div>
        {seconds === null ? null : (
          <p className="text-muted-foreground" aria-hidden="true">
            {t('stream.nextAttempt', {
              when: t('stream.retryIn', { seconds }),
            })}
          </p>
        )}
      </div>
      <div className="flex flex-wrap items-center gap-2">
        <Button variant="outline" size="sm" onClick={onRetry}>
          <ArrowClockwiseIcon className="size-3.5" aria-hidden="true" />
          {t('stream.retryNow')}
        </Button>
        <Button variant="outline" size="sm" onClick={onSwitchToPolling}>
          {t('stream.switchToPolling')}
        </Button>
        <a
          href={downloadUrl}
          download
          className={buttonVariants({ variant: 'outline', size: 'sm' })}
        >
          <DownloadSimpleIcon className="size-3.5" aria-hidden="true" />
          {t('stream.download')}
        </a>
      </div>
    </div>
  )
}
