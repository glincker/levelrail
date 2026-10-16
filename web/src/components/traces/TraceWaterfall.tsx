import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { cn } from '@/lib/utils'
import { formatDuration, layoutTrace } from '../../lib/traceTree'
import { useTrace } from '../../queries/traces'

const AXIS = 1000

/** TraceWaterfall draws one trace's spans as bars on a shared time axis. */
export function TraceWaterfall({ app, id }: { app: string; id: string }) {
  const { t } = useTranslation('traces')
  const { data, isPending, error } = useTrace(app, id)
  const layout = useMemo(() => layoutTrace(data ?? []), [data])

  if (isPending) {
    return (
      <p className="text-sm text-muted-foreground">{t('detail.loading')}</p>
    )
  }
  if (error) {
    return (
      <Alert variant="destructive">
        <AlertDescription>
          {t('list.loadError', { message: error.message })}
        </AlertDescription>
      </Alert>
    )
  }
  return (
    <ol className="space-y-1" aria-label={t('detail.title', { id })}>
      {layout.rows.map(({ span, depth, left, width }) => {
        const failed = span.status_code === 2
        return (
          <li
            key={span.span_id}
            className="grid grid-cols-[minmax(0,14rem)_1fr_4.5rem] items-center gap-3 text-xs"
          >
            <span
              className={cn(
                'truncate font-mono text-foreground',
                depth === 0 ? 'pl-0' : depth === 1 ? 'pl-3' : 'pl-6',
              )}
              title={span.name}
            >
              {span.name}
            </span>
            <svg
              viewBox={`0 0 ${AXIS} 12`}
              preserveAspectRatio="none"
              className="h-3 w-full rounded-sm bg-muted"
              role="img"
              aria-label={`${span.name} ${formatDuration(span.duration_ns)}`}
            >
              <rect
                x={left * AXIS}
                y={1}
                width={Math.max(width * AXIS, 2)}
                height={10}
                rx={2}
                className={failed ? 'fill-destructive' : 'fill-primary'}
              />
            </svg>
            <span className="text-right tabular-nums text-muted-foreground">
              {formatDuration(span.duration_ns)}
            </span>
          </li>
        )
      })}
    </ol>
  )
}
