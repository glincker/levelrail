import { useTranslation } from 'react-i18next'
import { Sparkline } from '@/components/kit/Sparkline'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import type { TrafficPoint } from '../../queries/pagesSites'

const nf = new Intl.NumberFormat()

function Row({
  label,
  hint,
  values,
  tone,
}: Readonly<{
  label: string
  hint: string
  values: number[]
  tone: 'neutral' | 'danger'
}>) {
  const total = values.reduce((n, v) => n + v, 0)
  return (
    <Tooltip>
      <TooltipTrigger
        render={<div />}
        className="flex items-center gap-2 text-xs"
      >
        <span className="w-14 shrink-0 text-muted-foreground">{label}</span>
        <span className="min-w-0 flex-1">
          <Sparkline
            values={values}
            tone={tone}
            width="fill"
            height={28}
            fill
            ariaLabel={`${label}: ${nf.format(total)}`}
          />
        </span>
        <span className="w-12 shrink-0 text-right tabular-nums">
          {nf.format(total)}
        </span>
      </TooltipTrigger>
      <TooltipContent>{hint}</TooltipContent>
    </Tooltip>
  )
}

export default function TrafficSeriesChart({
  series,
}: Readonly<{ series: TrafficPoint[] }>) {
  const { t } = useTranslation('pagesSites')
  const total = series.reduce((n, p) => n + p.requests, 0)
  if (total === 0) {
    return (
      <p className="text-xs text-muted-foreground">{t('traffic.noSeries')}</p>
    )
  }
  const errors = series.reduce((n, p) => n + p.errors, 0)
  return (
    <div className="space-y-1">
      <Row
        label={t('traffic.series.requests')}
        hint={t('traffic.series.requestsHint')}
        values={series.map((p) => p.requests)}
        tone="neutral"
      />
      <Row
        label={t('traffic.series.pages')}
        hint={t('traffic.series.pagesHint')}
        values={series.map((p) => p.pages)}
        tone="neutral"
      />
      {errors > 0 ? (
        <Row
          label={t('traffic.series.errors')}
          hint={t('traffic.series.errorsHint')}
          values={series.map((p) => p.errors)}
          tone="danger"
        />
      ) : null}
    </div>
  )
}
