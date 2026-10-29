import { useTranslation } from 'react-i18next'
import { ChartLineGlassIcon } from '@thesvg/ui-react'
import { Skeleton } from '@/components/ui/skeleton'
import { EmptyState } from '@/components/ui/empty-state'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { useAiUsage } from '../../queries/aiProviders'
import {
  formatCount,
  tokensPerDay,
  usageByModel,
  type DayPoint,
} from './aiProviderState'

function Sparkline({ points, label }: { points: DayPoint[]; label: string }) {
  const max = Math.max(1, ...points.map((p) => p.tokens))
  const step = 100 / Math.max(1, points.length - 1)
  const d = points
    .map(
      (p, i) =>
        `${i === 0 ? 'M' : 'L'}${(i * step).toFixed(2)},${(30 - (p.tokens / max) * 28 - 1).toFixed(2)}`,
    )
    .join(' ')
  return (
    <svg
      role="img"
      aria-label={label}
      viewBox="0 0 100 30"
      preserveAspectRatio="none"
      className="h-10 w-full text-tone-info"
    >
      <path
        d={d}
        fill="none"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinejoin="round"
        className="[vector-effect:non-scaling-stroke]"
      />
    </svg>
  )
}

export function AiUsagePanel({
  budget,
  monthTotal,
}: {
  budget?: number
  monthTotal?: number
}) {
  const { t } = useTranslation('settings')
  const usage = useAiUsage(30)

  if (usage.isPending) {
    return (
      <div className="space-y-2" aria-hidden="true">
        <Skeleton className="h-10 w-full" />
        <Skeleton className="h-16 w-full" />
      </div>
    )
  }
  const rows = usage.data?.days ?? []
  if (usage.isError || rows.length === 0) {
    return (
      <EmptyState
        compact
        icon={<ChartLineGlassIcon className="size-5.5" />}
        title={t('aiAssistantPage.usage.emptyTitle')}
        description={
          usage.isError
            ? t('aiAssistantPage.usage.unavailable')
            : t('aiAssistantPage.usage.emptyBody')
        }
      />
    )
  }
  const points = tokensPerDay(rows, 30)
  const total = monthTotal ?? points.reduce((sum, p) => sum + p.tokens, 0)
  const models = usageByModel(rows).slice(0, 8)
  const pct = budget ? Math.min(100, Math.round((total / budget) * 100)) : null
  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <p className="text-sm">
          {budget
            ? t('aiAssistantPage.usage.ofBudget', {
                used: formatCount(total),
                budget: formatCount(budget),
                pct,
              })
            : t('aiAssistantPage.usage.total', { used: formatCount(total) })}
        </p>
        <p className="text-xs text-muted-foreground">
          {t('aiAssistantPage.usage.last30')}
        </p>
      </div>
      <Sparkline points={points} label={t('aiAssistantPage.usage.sparkline')} />
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('aiAssistantPage.usage.model')}</TableHead>
            <TableHead className="text-right">
              {t('aiAssistantPage.usage.tokens')}
            </TableHead>
            <TableHead className="text-right">
              {t('aiAssistantPage.usage.requests')}
            </TableHead>
            <TableHead className="text-right">
              {t('aiAssistantPage.usage.errors')}
            </TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {models.map((m) => (
            <TableRow key={`${m.provider}/${m.model}`}>
              <TableCell>
                {m.provider}{' '}
                <span className="text-muted-foreground">{m.model}</span>
              </TableCell>
              <TableCell className="text-right tabular-nums">
                {formatCount(m.tokens)}
              </TableCell>
              <TableCell className="text-right tabular-nums">
                {m.requests}
              </TableCell>
              <TableCell className="text-right tabular-nums">
                {m.errors}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}
