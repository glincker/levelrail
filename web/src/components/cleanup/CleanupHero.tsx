import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { AnimatedNumber } from '../kit/AnimatedNumber'
import { formatRelative } from '../kit/formatRelative'
import { StatusPill } from '../kit/StatusPill'
import { TONE } from '../kit/tone'
import { formatSize } from '../../lib/format'
import {
  CATEGORY_STYLE,
  groupTargets,
  latestRun,
  percentOf,
  previewTargets,
  totalBytes,
} from '../../lib/cleanup'
import type {
  CleanupCategory,
  CleanupScheduler,
  CleanupTarget,
} from '../../types/cleanup'
import { UsageBar } from './UsageBar'

export function CleanupHero({
  targets,
  scheduler,
  filter,
  onFilter,
  onReview,
}: {
  targets: CleanupTarget[]
  scheduler: CleanupScheduler
  filter: CleanupCategory | null
  onFilter: (category: CleanupCategory | null) => void
  onReview: () => void
}) {
  const { t } = useTranslation('cleanup')
  const total = totalBytes(targets)
  const groups = groupTargets(targets)
  const last = latestRun(targets)
  const inPreview = previewTargets(targets).length

  const categoryLabel = (c: CleanupCategory) => t(`categories.${c}`)

  return (
    <section
      aria-label={t('hero.reclaimable')}
      className="space-y-4 rounded-lg border border-border bg-card p-4 sm:p-5"
    >
      <div className="flex flex-wrap items-start justify-between gap-x-6 gap-y-3">
        <div className="min-w-0">
          <p className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
            {t('hero.reclaimable')}
          </p>
          <p className="text-4xl font-semibold tracking-tight text-foreground sm:text-5xl">
            <AnimatedNumber value={total} format={(n) => formatSize(n)} />
          </p>
          <p className="text-sm text-muted-foreground">
            {t('hero.reclaimableHint')}
          </p>
        </div>
        <StatusPill
          tone={scheduler.enabled ? 'success' : 'neutral'}
          live={scheduler.enabled}
          size="md"
          label={
            scheduler.enabled
              ? t('hero.schedulerOn', { interval: scheduler.interval })
              : t('hero.schedulerOff')
          }
        />
      </div>

      {total > 0 ? (
        <div className="space-y-2">
          <UsageBar
            className="h-2.5"
            label={t('hero.usageLabel')}
            segments={groups.map((g) => ({
              key: g.category,
              percent: percentOf(g.bytes, total),
              tone: CATEGORY_STYLE[g.category].tone,
              faded: CATEGORY_STYLE[g.category].faded,
              dimmed: filter !== null && filter !== g.category,
              onSelect: () => {
                onFilter(filter === g.category ? null : g.category)
              },
            }))}
          />
          <ul className="flex flex-wrap gap-1.5">
            {groups.map((g) => {
              const style = CATEGORY_STYLE[g.category]
              const active = filter === g.category
              const percent = Math.round(percentOf(g.bytes, total))
              return (
                <li key={g.category}>
                  <button
                    type="button"
                    aria-pressed={active}
                    aria-label={`${t('hero.segmentLabel', {
                      category: categoryLabel(g.category),
                      size: formatSize(g.bytes),
                      percent,
                    })}. ${t('hero.legendFilter', {
                      category: categoryLabel(g.category),
                    })}`}
                    onClick={() => {
                      onFilter(active ? null : g.category)
                    }}
                    className={cn(
                      'inline-flex h-7 items-center gap-1.5 rounded-md border px-2 text-xs transition-colors outline-none focus-visible:ring-2 focus-visible:ring-ring pointer-coarse:min-h-11',
                      active
                        ? 'border-foreground/40 bg-muted text-foreground'
                        : 'border-border text-muted-foreground hover:bg-muted hover:text-foreground',
                    )}
                  >
                    <span
                      aria-hidden="true"
                      className={cn(
                        'size-2 rounded-full',
                        TONE[style.tone].solid,
                        style.faded && 'opacity-60',
                      )}
                    />
                    {categoryLabel(g.category)}
                    <span className="text-muted-foreground tabular-nums">
                      {formatSize(g.bytes)}
                    </span>
                  </button>
                </li>
              )
            })}
            {filter !== null ? (
              <li>
                <Button
                  size="xs"
                  variant="ghost"
                  onClick={() => {
                    onFilter(null)
                  }}
                >
                  {t('hero.clearFilter')}
                </Button>
              </li>
            ) : null}
          </ul>
        </div>
      ) : null}

      <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-2 border-t border-border pt-3 text-sm">
        <p className="text-muted-foreground">
          {last
            ? `${t('hero.lastRun')}: ${formatRelative(last)}`
            : t('hero.neverRun')}
        </p>
        <div className="flex flex-wrap items-center gap-3">
          <p
            className={cn(
              inPreview > 0 ? 'text-tone-warning' : 'text-muted-foreground',
            )}
          >
            {inPreview > 0
              ? t('hero.previewCount', { count: inPreview })
              : t('hero.previewNone')}
          </p>
          {inPreview > 0 ? (
            <Button size="sm" variant="outline" onClick={onReview}>
              {t('hero.reviewAction')}
            </Button>
          ) : null}
        </div>
      </div>
    </section>
  )
}
