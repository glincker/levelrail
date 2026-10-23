import { lazy, Suspense } from 'react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Banner } from '@/components/ui/banner'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Collapsible,
  CollapsiblePanel,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
  formatMetric,
  GROUP_ORDER,
  metricKeys,
  resultsByGroup,
  shortCommit,
} from '../../lib/benchmarkFormat'
import type { BenchmarkResult, BenchmarkRun } from '../../queries/benchmarks'
import type { ThroughputPoint } from './BenchmarkThroughputChart'

const ThroughputChart = lazy(() => import('./BenchmarkThroughputChart'))

const STATUS_VARIANT = {
  passed: 'success',
  failed: 'destructive',
  skipped: 'muted',
} as const

export function BenchmarkCaveats({ caveats }: { caveats: string[] }) {
  const { t } = useTranslation('resilience', { keyPrefix: 'benchmarks' })
  if (caveats.length === 0) return null
  return (
    <Banner variant="warning" title={t('caveatsTitle')}>
      <ul className="list-disc space-y-1 pl-5">
        {caveats.map((c) => (
          <li key={c}>{c}</li>
        ))}
      </ul>
    </Banner>
  )
}

function HostCard({ run }: { run: BenchmarkRun }) {
  const { t } = useTranslation('resilience', { keyPrefix: 'benchmarks.host' })
  const { host, build } = run.report
  const rows: [string, string][] = [
    [
      t('platform'),
      `${host.os}/${host.arch}${host.os_version ? ` ${host.os_version}` : ''}`,
    ],
    [t('cpu'), `${host.cpu_model ?? t('unknown')}, ${host.cpus}`],
    [
      t('memory'),
      host.mem_bytes
        ? `${Math.round(host.mem_bytes / (1 << 30))} GiB`
        : t('unknown'),
    ],
    [
      t('docker'),
      host.docker.version
        ? `${host.docker.version}${host.docker.os ? ` (${host.docker.os})` : ''}`
        : t('unknown'),
    ],
    [
      t('load'),
      t('loadValue', {
        start: host.load1_start.toFixed(2),
        end: host.load1_end.toFixed(2),
      }),
    ],
    [t('version'), build.version],
    [t('commit'), shortCommit(build.commit, build.dirty)],
    [t('runs'), String(run.report.repeat)],
  ]
  if (host.kernel) rows.splice(1, 0, [t('kernel'), host.kernel])
  if (host.note) rows.push([t('note'), host.note])
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('title')}</CardTitle>
      </CardHeader>
      <CardContent>
        <dl className="grid grid-cols-1 gap-x-8 gap-y-2 text-sm sm:grid-cols-2">
          {rows.map(([k, v]) => (
            <div key={k} className="flex flex-wrap justify-between gap-2">
              <dt className="text-muted-foreground">{k}</dt>
              <dd className="font-medium tabular-nums break-all">{v}</dd>
            </div>
          ))}
        </dl>
      </CardContent>
    </Card>
  )
}

function ResultTable({ results }: { results: BenchmarkResult[] }) {
  const { t } = useTranslation('resilience')
  const counts = new Map<string, number>()
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>{t('benchmarks.table.scenario')}</TableHead>
          <TableHead>{t('benchmarks.table.run')}</TableHead>
          <TableHead>{t('benchmarks.table.status')}</TableHead>
          <TableHead className="text-right">{t('metrics.recovery')}</TableHead>
          <TableHead className="text-right">{t('metrics.errors')}</TableHead>
          <TableHead>{t('benchmarks.table.summary')}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {results.map((r, i) => {
          const n = (counts.get(r.scenario) ?? 0) + 1
          counts.set(r.scenario, n)
          return (
            <TableRow key={`${r.scenario}-${i}`}>
              <TableCell className="font-medium">
                {t(`scenario.${r.scenario}`, { defaultValue: r.scenario })}
              </TableCell>
              <TableCell className="tabular-nums">{n}</TableCell>
              <TableCell>
                <Badge variant={STATUS_VARIANT[r.status]}>
                  {t(`status.${r.status}`)}
                </Badge>
              </TableCell>
              <TableCell className="text-right tabular-nums">
                {r.recovery_ms > 0 ? `${Math.round(r.recovery_ms)} ms` : '-'}
              </TableCell>
              <TableCell className="text-right tabular-nums">
                {r.request_errors} / {r.requests}
              </TableCell>
              <TableCell className="max-w-md text-muted-foreground">
                {r.summary}
              </TableCell>
            </TableRow>
          )
        })}
      </TableBody>
    </Table>
  )
}

function MetricTable({
  scenario,
  results,
}: {
  scenario: string
  results: BenchmarkResult[]
}) {
  const { t } = useTranslation('resilience')
  const keys = metricKeys(results)
  if (keys.length === 0) return null
  return (
    <div className="space-y-2">
      <h4 className="text-sm font-medium">
        {t(`scenario.${scenario}`, { defaultValue: scenario })}
      </h4>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('benchmarks.table.metric')}</TableHead>
            {results.map((_, i) => (
              <TableHead key={i} className="text-right">
                {t('benchmarks.table.runN', { n: i + 1 })}
              </TableHead>
            ))}
          </TableRow>
        </TableHeader>
        <TableBody>
          {keys.map((k) => (
            <TableRow key={k}>
              <TableCell className="font-mono text-xs">{k}</TableCell>
              {results.map((r, i) => (
                <TableCell key={i} className="text-right tabular-nums">
                  {formatMetric(k, r.metrics?.[k] ?? null)}
                </TableCell>
              ))}
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}

function throughputPoints(results: BenchmarkResult[]): ThroughputPoint[] {
  const first = results.find((r) => r.scenario === 'ingress-concurrency')
  if (!first?.metrics) return []
  const out: ThroughputPoint[] = []
  for (const [k, v] of Object.entries(first.metrics)) {
    const m = /^c(\d+)_rps$/.exec(k)
    if (m?.[1]) out.push({ label: m[1], rps: v })
  }
  return out.sort((a, b) => Number(a.label) - Number(b.label))
}

export function BenchmarkRunDetail({ run }: { run: BenchmarkRun }) {
  const { t } = useTranslation('resilience', { keyPrefix: 'benchmarks' })
  const { t: tr } = useTranslation('resilience')
  const grouped = resultsByGroup(run.report.results, run.groups)
  const points = throughputPoints(run.report.results)
  return (
    <div className="space-y-(--glinr-page-gap)">
      <BenchmarkCaveats caveats={run.report.caveats} />
      <HostCard run={run} />
      {GROUP_ORDER.map((g) => {
        const results = grouped.get(g) ?? []
        if (results.length === 0) return null
        const scenarios = [...new Set(results.map((r) => r.scenario))]
        return (
          <Card key={g}>
            <CardHeader>
              <CardTitle>{t(`groups.${g}`)}</CardTitle>
            </CardHeader>
            <CardContent className="space-y-6">
              <ResultTable results={results} />
              {g === 'throughput' && points.length > 0 ? (
                <Suspense fallback={<Skeleton className="h-48 w-full" />}>
                  <ThroughputChart
                    points={points}
                    title={tr('scenario.ingress-concurrency')}
                  />
                </Suspense>
              ) : null}
              {scenarios.map((s) => (
                <MetricTable
                  key={s}
                  scenario={s}
                  results={results.filter((r) => r.scenario === s)}
                />
              ))}
            </CardContent>
          </Card>
        )
      })}
      <Card>
        <CardHeader>
          <CardTitle>{t('limits.title')}</CardTitle>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('limits.question')}</TableHead>
                <TableHead>{t('limits.status')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {run.report.limits.map((l) => (
                <TableRow key={l.question}>
                  <TableCell>{l.question}</TableCell>
                  <TableCell className="text-muted-foreground">
                    {l.status}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
      <Collapsible>
        <CollapsibleTrigger className="text-sm text-muted-foreground underline">
          {t('method')}
        </CollapsibleTrigger>
        <CollapsiblePanel>
          <ul className="mt-2 list-disc space-y-1 pl-5 text-sm text-muted-foreground">
            {run.report.methodology.map((m) => (
              <li key={m}>{m}</li>
            ))}
          </ul>
        </CollapsiblePanel>
      </Collapsible>
    </div>
  )
}
