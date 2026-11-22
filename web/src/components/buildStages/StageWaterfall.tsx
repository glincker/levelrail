import {
  CircleAlertGlassIcon,
  CircleCheckGlassIcon,
  LoaderCircleGlassIcon,
  type UiIcon,
} from '@thesvg/ui-react'
import { useDeployDetailT } from '../deployDetail/useDeployDetailT'
import { formatDurationMs } from '../../lib/deployDuration'
import {
  buildWaterfall,
  cacheChipState,
  wallGap,
  type BuildStagesResponse,
  type BuildStageName,
  type BuildStageStatus,
  type WallGap,
} from '../../lib/buildStages'
import { CacheChip } from './CacheChip'

const BAR_CLASS = {
  running: 'fill-primary',
  done: 'fill-tone-success',
  failed: 'fill-tone-danger',
} as const

const ICON: Record<BuildStageStatus, UiIcon> = {
  running: LoaderCircleGlassIcon,
  done: CircleCheckGlassIcon,
  failed: CircleAlertGlassIcon,
}

const ICON_CLASS: Record<BuildStageStatus, string> = {
  running: 'text-primary animate-spin motion-reduce:animate-none',
  done: 'text-tone-success',
  failed: 'text-tone-danger',
}

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

/** One compact row per stage on a shared time axis; a row selects its log. */
export function StageWaterfall({
  data,
  nowMs,
  selected = null,
  onSelect,
}: {
  data: BuildStagesResponse
  nowMs: number
  selected?: BuildStageName | null
  onSelect?: (stage: BuildStageName) => void
}) {
  const { t } = useDeployDetailT()
  const wf = buildWaterfall(data, nowMs)
  const running = data.stages.some((s) => s.status === 'running' && !s.ended_at)
  // Summed here so a running stage counts its elapsed time; the server total is 0 until it ends.
  const stagesMs = wf.rows.reduce((sum, r) => sum + r.durationMs, 0)
  const gap = running ? '' : gapText(wallGap(data), t)
  return (
    <section
      aria-label={t('buildStages.waterfallLabel')}
      className="space-y-1.5"
    >
      <ul className="divide-y divide-border/60">
        {wf.rows.map((row) => {
          const stage = data.stages.find((s) => s.stage === row.stage)
          const Icon = ICON[row.status]
          const name = t(`buildStages.stage.${row.stage}`)
          const duration = formatDurationMs(row.durationMs) ?? '-'
          return (
            <li
              key={row.stage}
              data-testid={`waterfall-${row.stage}`}
              className="grid grid-cols-[6.5rem_1fr_auto] items-center gap-2 py-1 text-xs sm:grid-cols-[8rem_1fr_auto]"
            >
              <button
                type="button"
                aria-pressed={selected === row.stage}
                aria-label={t('buildStages.nodeLabel', {
                  stage: name,
                  status: t(`buildStages.status.${row.status}`),
                  duration,
                })}
                data-status={row.status}
                onClick={() => {
                  onSelect?.(row.stage)
                }}
                className={`flex min-w-0 items-center gap-1.5 rounded px-1 py-0.5 text-left text-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none ${selected === row.stage ? 'bg-muted' : 'hover:bg-muted/60'}`}
              >
                <Icon
                  className={`size-3.5 shrink-0 ${ICON_CLASS[row.status]}`}
                  aria-hidden="true"
                />
                <span className="truncate">{name}</span>
              </button>
              <svg
                viewBox="0 0 100 6"
                preserveAspectRatio="none"
                role="img"
                aria-label={t('buildStages.barLabel', {
                  stage: name,
                  percent: Math.round(row.widthPct),
                })}
                className="h-2.5 w-full rounded-sm bg-muted"
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
              <span className="flex items-center justify-end gap-2">
                {stage ? <CacheChip state={cacheChipState(stage)} /> : null}
                <span className="w-14 text-right font-mono text-muted-foreground">
                  {duration}
                </span>
              </span>
            </li>
          )
        })}
      </ul>
      <p
        data-testid="waterfall-totals"
        className="text-xs text-muted-foreground"
      >
        {running
          ? t('buildStages.totalsRunning', {
              wall: formatDurationMs(wf.axisMs) ?? '-',
              stages: formatDurationMs(stagesMs) ?? '-',
            })
          : t('buildStages.totals', {
              wall: formatDurationMs(data.wall_ms) ?? '-',
              stages: formatDurationMs(stagesMs) ?? '-',
            })}
        {gap ? ` ${gap}` : ''}
      </p>
    </section>
  )
}
