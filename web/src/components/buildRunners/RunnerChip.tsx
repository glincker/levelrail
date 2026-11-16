import { useTranslation } from 'react-i18next'
import { ServerGlassIcon, ClockGlassIcon } from '@thesvg/ui-react'
import { formatWaitMs } from '../../lib/buildRunnerFormat'
import type { BuildRunnerInfo } from '../../queries/buildRunners'

// Where the build ran and how long it queued; nothing when the row has neither.
export function RunnerChip({
  info,
}: Readonly<{ info?: BuildRunnerInfo | null }>) {
  const { t } = useTranslation('deploys')
  if (!info) return null
  const name = info.runner_name
  const wait = info.queue_wait_ms ?? 0
  if (!name && wait <= 0) return null
  return (
    <>
      {name ? (
        <span
          className="inline-flex items-center gap-1.5"
          title={info.runner_reason}
        >
          <ServerGlassIcon
            className="size-5 text-muted-foreground/70"
            aria-hidden="true"
          />
          <span className="text-foreground">
            {t('runners.builtOn', { name })}
          </span>
          {info.runner_reason ? (
            <span className="sr-only">{info.runner_reason}</span>
          ) : null}
        </span>
      ) : null}
      {wait > 0 ? (
        <span className="inline-flex items-center gap-1.5">
          <ClockGlassIcon
            className="size-5 text-muted-foreground/70"
            aria-hidden="true"
          />
          {t('runners.queued', { wait: formatWaitMs(wait) })}
        </span>
      ) : null}
    </>
  )
}
