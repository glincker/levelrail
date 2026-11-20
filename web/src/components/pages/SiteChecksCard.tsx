import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { CircleCheckGlassIcon, TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { ProgressRing } from '@/components/ui/progress-ring'
import { Skeleton } from '@/components/ui/skeleton'
import {
  siteChecksKey,
  siteChecksQueryOptions,
  type SiteCheck,
} from '../../queries/pagesSites'
import { formatSize } from '../../lib/format'

function CheckRow({ check }: Readonly<{ check: SiteCheck }>) {
  const { t } = useTranslation('pagesSites')
  const ok = check.status === 'ok'
  return (
    <li className="flex items-start gap-2 text-sm">
      {ok ? (
        <CircleCheckGlassIcon
          className="mt-0.5 size-4 shrink-0 text-emerald-600 dark:text-emerald-400"
          aria-hidden="true"
        />
      ) : (
        <TriangleAlertGlassIcon
          className={
            check.status === 'fail'
              ? 'mt-0.5 size-4 shrink-0 text-destructive'
              : 'mt-0.5 size-4 shrink-0 text-amber-600 dark:text-amber-400'
          }
          aria-hidden="true"
        />
      )}
      <div className="min-w-0">
        <p className="font-medium">
          {check.title}
          <span className="sr-only">
            {' '}
            ({t(`checks.status.${check.status}`)})
          </span>
        </p>
        {check.detail ? (
          <p className="text-xs text-muted-foreground">{check.detail}</p>
        ) : null}
        {check.path ? (
          <code className="text-xs text-muted-foreground">{check.path}</code>
        ) : null}
      </div>
    </li>
  )
}

/** Score ring, problems first, passing checks folded away. */
export function SiteChecksCard({ name }: Readonly<{ name: string }>) {
  const { t } = useTranslation('pagesSites')
  const queryClient = useQueryClient()
  const q = useQuery(siteChecksQueryOptions(name))
  const rerun = () =>
    void queryClient.invalidateQueries({ queryKey: siteChecksKey(name) })
  const report = q.data
  const problems = report?.checks.filter((c) => c.status !== 'ok') ?? []
  problems.sort(
    (a, b) => Number(b.status === 'fail') - Number(a.status === 'fail'),
  )
  const passing = report?.checks.filter((c) => c.status === 'ok') ?? []

  return (
    <section
      aria-labelledby="pages-checks-title"
      className="space-y-3 rounded-lg border border-border bg-card p-3"
    >
      <div className="flex items-center gap-3">
        {report ? (
          <ProgressRing value={report.score} label={t('checks.score')} />
        ) : null}
        <div className="min-w-0 flex-1">
          <h2 id="pages-checks-title" className="text-sm font-semibold">
            {t('checks.title')}
          </h2>
          {report ? (
            <p className="text-xs text-muted-foreground">
              {t('checks.scanned', {
                count: report.files_scanned,
                size: formatSize(report.total_bytes),
              })}
              {report.truncated ? ` ${t('checks.truncated')}` : ''}
            </p>
          ) : null}
        </div>
        <Button
          size="sm"
          variant="outline"
          disabled={q.isFetching}
          onClick={rerun}
        >
          {q.isFetching ? t('checks.running') : t('checks.rerun')}
        </Button>
      </div>
      {q.isPending ? <Skeleton className="h-16 w-full" /> : null}
      {q.isError ? (
        <Alert variant="destructive" role="alert">
          <AlertDescription>{t('checks.failed')}</AlertDescription>
        </Alert>
      ) : null}
      {report && problems.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t('checks.allGood')}</p>
      ) : null}
      {problems.length > 0 ? (
        <ul className="space-y-2">
          {problems.map((c) => (
            <CheckRow key={c.id} check={c} />
          ))}
        </ul>
      ) : null}
      {passing.length > 0 ? (
        <details className="text-sm">
          <summary className="cursor-pointer text-xs text-muted-foreground focus-visible:underline">
            {t('checks.passing', { count: passing.length })}
          </summary>
          <ul className="mt-2 space-y-2">
            {passing.map((c) => (
              <CheckRow key={c.id} check={c} />
            ))}
          </ul>
        </details>
      ) : null}
    </section>
  )
}
