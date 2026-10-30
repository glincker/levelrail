import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ChartLineGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import { usePipelineInsights } from '../queries/pipelineUx'
import type { Pipeline } from '../types/pipelines'

function secs(n: number): string {
  if (n < 60) {
    return `${n}s`
  }
  return `${Math.floor(n / 60)}m ${n % 60}s`
}

export function PipelineInsightsCard({
  appName,
  pipelines,
}: {
  appName: string
  pipelines: Pipeline[]
}) {
  const { t } = useTranslation('pipelines')
  const [picked, setPicked] = useState('')
  const name = picked || pipelines[0]?.name || ''
  const { data, isLoading, error } = usePipelineInsights(appName, name)
  if (pipelines.length === 0) {
    return null
  }
  return (
    <section className="rounded-lg border border-border p-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 className="flex items-center gap-1.5 text-sm font-semibold text-foreground">
          <ChartLineGlassIcon
            className="size-5 text-muted-foreground"
            aria-hidden="true"
          />
          {t('insights.title')}
        </h2>
        {pipelines.length > 1 ? (
          <select
            aria-label={t('insights.pipeline')}
            value={name}
            onChange={(e) => setPicked(e.target.value)}
            className="h-8 rounded-md border border-input bg-background px-2 text-sm"
          >
            {pipelines.map((p) => (
              <option key={p.id} value={p.name}>
                {p.name}
              </option>
            ))}
          </select>
        ) : null}
      </div>
      <div className="mt-3 space-y-4">
        {isLoading ? (
          <Skeleton className="h-16 w-full" />
        ) : error ? (
          <p className="text-sm text-destructive">{t('insights.loadFailed')}</p>
        ) : !data || data.jobs.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t('insights.empty')}</p>
        ) : (
          <>
            <table className="w-full text-sm">
              <thead className="text-left text-xs text-muted-foreground">
                <tr>
                  <th className="py-1 font-medium">{t('insights.job')}</th>
                  <th className="py-1 font-medium">{t('insights.runs')}</th>
                  <th className="py-1 font-medium">{t('insights.failRate')}</th>
                  <th className="py-1 font-medium">{t('insights.p50')}</th>
                  <th className="py-1 font-medium">{t('insights.last')}</th>
                  <th className="py-1" />
                </tr>
              </thead>
              <tbody>
                {data.jobs.map((j) => (
                  <tr
                    key={j.job}
                    className="border-t border-border tabular-nums"
                  >
                    <td className="py-1.5 font-medium text-foreground">
                      {j.job}
                    </td>
                    <td className="py-1.5">{j.runs}</td>
                    <td className="py-1.5">
                      {Math.round(j.failure_rate * 100)}%
                    </td>
                    <td className="py-1.5">{secs(j.p50_seconds)}</td>
                    <td className="py-1.5">{secs(j.last_seconds)}</td>
                    <td className="py-1.5">
                      {j.flaky ? (
                        <Badge variant="warning" title={j.flaky_reason}>
                          {t('insights.flaky')}
                        </Badge>
                      ) : null}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
            {data.diff.length > 0 ? (
              <div>
                <p className="mb-1 text-xs font-medium text-muted-foreground">
                  {t('insights.diffTitle')}
                </p>
                <table className="w-full text-sm">
                  <thead className="text-left text-xs text-muted-foreground">
                    <tr>
                      <th className="py-1 font-medium">{t('insights.job')}</th>
                      <th className="py-1 font-medium">
                        {t('insights.before')}
                      </th>
                      <th className="py-1 font-medium">{t('insights.now')}</th>
                      <th className="py-1 font-medium">
                        {t('insights.change')}
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    {data.diff.map((d) => (
                      <tr
                        key={d.job}
                        className="border-t border-border tabular-nums"
                      >
                        <td className="py-1.5 font-medium text-foreground">
                          {d.job}
                        </td>
                        <td className="py-1.5">{secs(d.baseline_seconds)}</td>
                        <td className="py-1.5">{secs(d.current_seconds)}</td>
                        <td
                          className={
                            d.delta_percent > 20
                              ? 'py-1.5 text-destructive'
                              : 'py-1.5 text-muted-foreground'
                          }
                        >
                          {d.delta_seconds > 0 ? '+' : ''}
                          {d.delta_seconds}s ({Math.round(d.delta_percent)}%)
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            ) : null}
          </>
        )}
      </div>
    </section>
  )
}
