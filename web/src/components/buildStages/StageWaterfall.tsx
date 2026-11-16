import { useDeployDetailT } from '../deployDetail/useDeployDetailT'
import { formatDurationMs } from '../../lib/deployDuration'
import {
  buildWaterfall,
  wallGap,
  type BuildStagesResponse,
  type WallGap,
} from '../../lib/buildStages'

const BAR_CLASS = {
  running: 'fill-primary',
  done: 'fill-tone-success',
  failed: 'fill-tone-danger',
} as const

// Bars in SVG user units (0..100) so widths need no inline style.
const MIN_BAR_PCT = 0.5

function gapText(
  gap: WallGap,
  t: (key: string, v?: Record<string, string | number>) => string,
): string {
  if (gap.kind === 'match') return ''
  return t(`buildStages.gap.${gap.kind}`, {
    time: formatDurationMs(gap.ms) ?? '-',
  })
}

/** Stage bars on one time axis, with the stage sum shown beside wall time. */
export function StageWaterfall({
  data,
  nowMs,
}: {
  data: BuildStagesResponse
  nowMs: number
}) {
  const { t } = useDeployDetailT()
  const wf = buildWaterfall(data, nowMs)
  const gap = gapText(wallGap(data), t)
  return (
    <section aria-label={t('buildStages.waterfallLabel')} className="space-y-2">
      <ul className="space-y-1">
        {wf.rows.map((row) => (
          <li
            key={row.stage}
            data-testid={`waterfall-${row.stage}`}
            className="grid grid-cols-[5rem_1fr_4rem] items-center gap-2 text-xs sm:grid-cols-[6rem_1fr_5rem]"
          >
            <span className="truncate text-foreground">
              {t(`buildStages.stage.${row.stage}`)}
            </span>
            <svg
              viewBox="0 0 100 6"
              preserveAspectRatio="none"
              role="img"
              aria-label={t('buildStages.barLabel', {
                stage: t(`buildStages.stage.${row.stage}`),
                percent: Math.round(row.widthPct),
              })}
              className="h-3 w-full rounded-sm bg-muted"
            >
              <rect
                x={row.leftPct}
                y={0}
                width={Math.max(
                  MIN_BAR_PCT,
                  Math.min(row.widthPct, 100 - row.leftPct),
                )}
                height={6}
                className={`${BAR_CLASS[row.status]} ${row.status === 'running' ? 'animate-pulse motion-reduce:animate-none' : ''}`}
              />
            </svg>
            <span className="text-right font-mono text-muted-foreground">
              {formatDurationMs(row.durationMs) ?? '-'}
            </span>
          </li>
        ))}
      </ul>
      <p
        data-testid="waterfall-totals"
        className="text-xs text-muted-foreground"
      >
        {t('buildStages.totals', {
          wall: formatDurationMs(data.wall_ms) ?? '-',
          stages: formatDurationMs(data.stages_ms) ?? '-',
        })}
        {gap ? ` ${gap}` : ''}
      </p>
    </section>
  )
}
