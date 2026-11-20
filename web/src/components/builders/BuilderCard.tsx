import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import { CpuGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Progress } from '@/components/ui/progress'
import { StatusBadge } from '@/components/ui/status-badge'
import { RelativeTime } from '@/components/kit'
import { TONE } from '@/components/kit/tone'
import { cn } from '@/lib/utils'
import {
  builderState,
  formatDurationMs,
  isBuilderPressure,
  loadPercent,
  osArchLabel,
  pressureTone,
} from '../../lib/builderFormat'
import type { Builder } from '../../queries/builders'
import { BuildTierSelect } from './BuildTierSelect'

export function PressureDot({ pressure }: Readonly<{ pressure: string }>) {
  const { t } = useTranslation('nodes')
  const key = isBuilderPressure(pressure) ? pressure : 'unknown'
  return (
    <span
      className={cn(
        'inline-flex items-center gap-1 text-xs',
        TONE[pressureTone(key)].text,
      )}
    >
      <span
        aria-hidden="true"
        className={cn('size-1.5 rounded-full', TONE[pressureTone(key)].solid)}
      />
      {t('builders.pressure.label', { level: t(`builders.pressure.${key}`) })}
    </span>
  )
}

// One card on wide screens; the same markup wraps into a compact row on narrow ones.
export function BuilderCard({ builder: b }: Readonly<{ builder: Builder }>) {
  const { t } = useTranslation('nodes')
  const { t: td } = useTranslation('deploys')
  const pct = loadPercent(b.slots_used, b.slots_total)
  const platform = osArchLabel(b.os, b.arch)
  return (
    <li className="flex flex-wrap items-center gap-x-3 gap-y-2 rounded-lg border border-border bg-card p-3 text-sm">
      <div className="flex min-w-0 flex-1 basis-40 flex-col gap-1">
        <Link
          to="/nodes/$id"
          params={{ id: b.id }}
          className="truncate font-medium"
        >
          {b.name}
        </Link>
        <div className="flex flex-wrap items-center gap-2">
          <StatusBadge
            form="dot"
            state={builderState(b.status)}
            label={t(`builders.status.${b.status}`, {
              defaultValue: td(`runners.status.${b.status}`, {
                defaultValue: b.status,
              }),
            })}
          />
          {platform ? (
            <Badge variant="muted">
              <CpuGlassIcon aria-hidden="true" size="1.2em" />
              {platform}
            </Badge>
          ) : null}
        </div>
      </div>
      <BuildTierSelect
        id={`builder-tier-${b.id}`}
        nodeId={b.id}
        value={b.tier}
        label={t('builders.tierFor', { name: b.name })}
        className="w-32"
      />
      <dl className="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-muted-foreground">
        <div className="flex items-center gap-1.5">
          <dt className="sr-only">{t('builders.col.builds')}</dt>
          <dd className="flex items-center gap-1.5 tabular-nums">
            <Progress
              value={pct}
              aria-label={t('builders.load', {
                used: b.slots_used,
                total: b.slots_total,
              })}
              className="w-16"
            />
            {t('builders.buildsNow', {
              used: b.slots_used,
              total: b.slots_total,
            })}
          </dd>
        </div>
        <div>
          <dt className="sr-only">{t('builders.col.avg')}</dt>
          <dd className="tabular-nums">
            {t('builders.avg', { value: formatDurationMs(b.avg_build_ms) })}
          </dd>
        </div>
        <div>
          <dt className="sr-only">{t('builders.col.pressure')}</dt>
          <dd>
            <PressureDot pressure={b.pressure} />
          </dd>
        </div>
        <div>
          <dt className="sr-only">{t('builders.col.lastSeen')}</dt>
          <dd>
            {b.last_seen_at ? (
              <RelativeTime at={b.last_seen_at} />
            ) : (
              t('builders.never')
            )}
          </dd>
        </div>
      </dl>
    </li>
  )
}
