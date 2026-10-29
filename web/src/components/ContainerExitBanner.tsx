import { useTranslation } from 'react-i18next'
import { CircleStopGlassIcon } from '@thesvg/ui-react'
export interface ContainerExitBannerProps {
  exitCode?: number
  oomKilled?: boolean
  finishedAt?: string
  /** The reconciler's own explanation, e.g. "Container exited with code 0
   *  after 2s, before it became ready". */
  message?: string
  reason?: string
  /** Show the hint for images that print help and exit 0. */
  showHint?: boolean
}

type HeadlineKey =
  | 'containerExit.exitedAt'
  | 'containerExit.exited'
  | 'containerExit.oomAt'
  | 'containerExit.oom'

function headlineKey(oom: boolean, hasTime: boolean): HeadlineKey {
  if (oom) return hasTime ? 'containerExit.oomAt' : 'containerExit.oom'
  return hasTime ? 'containerExit.exitedAt' : 'containerExit.exited'
}

// One shared statement of how a container stopped, used by the live logs
// page, the app overview and the deploy page so all three read the same.
export function ContainerExitBanner({
  exitCode,
  oomKilled = false,
  finishedAt,
  message,
  reason,
  showHint = false,
}: ContainerExitBannerProps) {
  const { t } = useTranslation('observability')
  const time = finishedAt ? new Date(finishedAt).toLocaleString() : ''
  return (
    <div
      role="status"
      className="flex items-start gap-2.5 rounded-lg border border-tone-danger-border bg-tone-danger-soft p-3 text-tone-danger"
    >
      <CircleStopGlassIcon
        className="mt-0.5 size-4.5 shrink-0"
        aria-hidden="true"
      />
      <div className="min-w-0 flex-1 text-sm">
        <p className="font-medium">
          {t(headlineKey(oomKilled, time !== ''), {
            code: exitCode ?? 0,
            time,
          })}
        </p>
        {message ? (
          <p className="mt-0.5 break-words text-foreground">
            {message}
            {reason ? (
              <span className="ml-1.5 font-mono text-xs text-muted-foreground">
                {reason}
              </span>
            ) : null}
          </p>
        ) : null}
        {showHint && exitCode === 0 && !oomKilled ? (
          <p className="mt-1 text-xs text-muted-foreground">
            {t('containerExit.hint')}
          </p>
        ) : null}
      </div>
    </div>
  )
}
