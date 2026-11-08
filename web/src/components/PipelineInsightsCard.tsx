import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ChartLineGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
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
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('insights.job')}</TableHead>
                  <TableHead>{t('insights.runs')}</TableHead>
                  <TableHead>{t('insights.failRate')}</TableHead>
                  <TableHead>{t('insights.p50')}</TableHead>
                  <TableHead>{t('insights.last')}</TableHead>
                  <TableHead />
                </TableRow>
              </TableHeader>
              <TableBody>
                {data.jobs.map((j) => (
                  <TableRow key={j.job} className="tabular-nums">
                    <TableCell className="font-medium text-foreground">
                      {j.job}
                    </TableCell>
                    <TableCell>{j.runs}</TableCell>
                    <TableCell>{Math.round(j.failure_rate * 100)}%</TableCell>
                    <TableCell>{secs(j.p50_seconds)}</TableCell>
                    <TableCell>{secs(j.last_seconds)}</TableCell>
                    <TableCell>
                      {j.flaky ? (
                        <Badge variant="warning" title={j.flaky_reason}>
                          {t('insights.flaky')}
                        </Badge>
                      ) : null}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
            {data.diff.length > 0 ? (
              <div>
                <p className="mb-1 text-xs font-medium text-muted-foreground">
                  {t('insights.diffTitle')}
                </p>
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>{t('insights.job')}</TableHead>
                      <TableHead>{t('insights.before')}</TableHead>
                      <TableHead>{t('insights.now')}</TableHead>
                      <TableHead>{t('insights.change')}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {data.diff.map((d) => (
                      <TableRow key={d.job} className="tabular-nums">
                        <TableCell className="font-medium text-foreground">
                          {d.job}
                        </TableCell>
                        <TableCell>{secs(d.baseline_seconds)}</TableCell>
                        <TableCell>{secs(d.current_seconds)}</TableCell>
                        <TableCell
                          className={
                            d.delta_percent > 20
                              ? 'text-destructive'
                              : 'text-muted-foreground'
                          }
                        >
                          {d.delta_seconds > 0 ? '+' : ''}
                          {d.delta_seconds}s ({Math.round(d.delta_percent)}%)
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            ) : null}
          </>
        )}
      </div>
    </section>
  )
}
