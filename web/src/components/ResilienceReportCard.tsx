import { useTranslation } from 'react-i18next'
import { RefreshCwGlassIcon, ShieldCheckGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { ListSkeleton } from '@/components/ui/list-skeleton'
import { useBrand } from '../hooks/useBrand'
import { cliCommand } from '../lib/cliCommand'
import { useResilienceReport } from '../queries/resilience'
import type { ChaosResult } from '../queries/resilience'

const STATUS_VARIANT = {
  passed: 'success',
  failed: 'destructive',
  skipped: 'muted',
} as const

function formatMs(ms: number): string {
  return ms > 0 ? `${Math.round(ms)} ms` : '-'
}

function ResultRow({ result }: { result: ChaosResult }) {
  const { t } = useTranslation('resilience')
  return (
    <li className="space-y-1 py-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <span className="text-sm font-medium">
          {t(`scenario.${result.scenario}`, { defaultValue: result.scenario })}
        </span>
        <Badge variant={STATUS_VARIANT[result.status]}>
          {t(`status.${result.status}`)}
        </Badge>
      </div>
      <p className="text-sm text-muted-foreground">{result.summary}</p>
      <dl className="flex flex-wrap gap-x-6 gap-y-1 text-xs text-muted-foreground">
        <div className="flex gap-1">
          <dt>{t('metrics.recovery')}</dt>
          <dd className="text-foreground">{formatMs(result.recovery_ms)}</dd>
        </div>
        <div className="flex gap-1">
          <dt>{t('metrics.errors')}</dt>
          <dd className="text-foreground">
            {result.request_errors} / {result.requests}
          </dd>
        </div>
        <div className="flex gap-1">
          <dt>{t('metrics.ranAt')}</dt>
          <dd className="text-foreground">
            {new Date(result.ran_at).toLocaleString()}
          </dd>
        </div>
      </dl>
    </li>
  )
}

/** ResilienceReportCard shows the last recorded chaos result per scenario, read only. */
export function ResilienceReportCard() {
  const { t } = useTranslation('resilience')
  const brand = useBrand()
  const { data, isLoading, isError, isFetching, refetch } =
    useResilienceReport()

  return (
    <Card>
      <CardHeader>
        <div className="flex items-start justify-between gap-3">
          <div className="space-y-1">
            <CardTitle className="flex items-center gap-2">
              <ShieldCheckGlassIcon
                className="text-tone-success"
                size="1.4em"
                aria-hidden="true"
              />
              {t('title')}
            </CardTitle>
            <CardDescription>{t('description')}</CardDescription>
          </div>
          <Button
            variant="outline"
            size="sm"
            onClick={() => void refetch()}
            disabled={isFetching}
          >
            <RefreshCwGlassIcon size="1.4em" aria-hidden="true" />
            {t('retry')}
          </Button>
        </div>
      </CardHeader>
      <CardContent className="space-y-3">
        {isLoading && <ListSkeleton rows={3} />}
        {isError && (
          <Alert variant="destructive">
            <AlertDescription>{t('loadError')}</AlertDescription>
          </Alert>
        )}
        {data && data.latest.length === 0 && (
          <div className="space-y-1">
            <p className="text-sm font-medium">{t('empty.title')}</p>
            <p className="text-sm text-muted-foreground">{t('empty.body')}</p>
          </div>
        )}
        {data && data.latest.length > 0 && (
          <ul className="divide-y divide-border">
            {data.latest.map((r) => (
              <ResultRow key={r.scenario} result={r} />
            ))}
          </ul>
        )}
        {data && data.not_yet_measured.length > 0 && (
          <ul className="space-y-1 text-sm text-muted-foreground">
            {data.not_yet_measured.map((name) => (
              <li
                key={name}
                className="flex items-center justify-between gap-2"
              >
                <span>{t(`scenario.${name}`, { defaultValue: name })}</span>
                <Badge variant="muted">{t('notMeasured')}</Badge>
              </li>
            ))}
          </ul>
        )}
        <details className="text-xs text-muted-foreground">
          <summary className="cursor-pointer">{t('cli.summary')}</summary>
          <p className="mt-2">
            <code>
              {cliCommand(brand.BinaryName, 'bench chaos run all --record')}
            </code>
          </p>
          <p>{t('cli.note')}</p>
        </details>
      </CardContent>
    </Card>
  )
}
