import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Banner } from '@/components/ui/banner'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
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
  formatPct,
  GROUP_ORDER,
  shortCommit,
} from '../../lib/benchmarkFormat'
import type {
  BenchmarkComparison,
  BenchmarkDelta,
  DeltaDirection,
} from '../../queries/benchmarks'

const DIRECTION_VARIANT = {
  better: 'success',
  worse: 'destructive',
  same: 'muted',
  '': 'muted',
} as const satisfies Record<DeltaDirection, string>

export function BenchmarkCompare({ data }: { data: BenchmarkComparison }) {
  const { t } = useTranslation('resilience')
  return (
    <div className="space-y-(--glinr-page-gap)">
      {data.warnings.length > 0 ? (
        <Banner variant="warning" title={t('benchmarks.compare.warningsTitle')}>
          <ul className="list-disc space-y-1 pl-5">
            {data.warnings.map((w) => (
              <li key={w}>{w}</li>
            ))}
          </ul>
        </Banner>
      ) : null}
      <Card>
        <CardHeader>
          <CardTitle>{t('benchmarks.compare.title')}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-2 text-sm">
          {(['base', 'head'] as const).map((side) => {
            const s = data[side]
            return (
              <p key={side}>
                <span className="font-medium">
                  {t(`benchmarks.compare.${side}`)}
                </span>{' '}
                <span className="font-mono text-xs">{s.id}</span>{' '}
                <span className="text-muted-foreground">
                  {s.version}, {shortCommit(s.commit, s.dirty)}, {s.host},{' '}
                  {t('benchmarks.compare.runs', { count: s.runs })}
                </span>
              </p>
            )
          })}
          <p className="text-xs text-muted-foreground">
            {t('benchmarks.compare.basis', { noise: data.noise_pct })}
          </p>
        </CardContent>
      </Card>
      {data.status_changes.length > 0 ? (
        <Card>
          <CardHeader>
            <CardTitle>{t('benchmarks.compare.statusTitle')}</CardTitle>
          </CardHeader>
          <CardContent>
            <ul className="space-y-1 text-sm">
              {data.status_changes.map((c) => (
                <li key={c.scenario}>
                  {t(`scenario.${c.scenario}`, { defaultValue: c.scenario })}
                  {': '}
                  {c.base} {t('benchmarks.compare.to')} {c.head}
                </li>
              ))}
            </ul>
          </CardContent>
        </Card>
      ) : null}
      {GROUP_ORDER.map((g) => {
        const deltas = data.deltas.filter((d) => d.group === g)
        if (deltas.length === 0) return null
        return (
          <Card key={g}>
            <CardHeader>
              <CardTitle>{t(`benchmarks.groups.${g}`)}</CardTitle>
            </CardHeader>
            <CardContent>
              <DeltaTable deltas={deltas} />
            </CardContent>
          </Card>
        )
      })}
    </div>
  )
}

function DeltaTable({ deltas }: { deltas: BenchmarkDelta[] }) {
  const { t } = useTranslation('resilience')
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>{t('benchmarks.table.scenario')}</TableHead>
          <TableHead>{t('benchmarks.table.metric')}</TableHead>
          <TableHead className="text-right">
            {t('benchmarks.compare.base')}
          </TableHead>
          <TableHead className="text-right">
            {t('benchmarks.compare.head')}
          </TableHead>
          <TableHead className="text-right">
            {t('benchmarks.compare.delta')}
          </TableHead>
          <TableHead>{t('benchmarks.compare.verdict')}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {deltas.map((d) => (
          <TableRow key={`${d.scenario}-${d.metric}`}>
            <TableCell>
              {t(`scenario.${d.scenario}`, { defaultValue: d.scenario })}
            </TableCell>
            <TableCell className="font-mono text-xs">{d.metric}</TableCell>
            <TableCell className="text-right tabular-nums">
              {formatMetric(d.metric, d.base)}
            </TableCell>
            <TableCell className="text-right tabular-nums">
              {formatMetric(d.metric, d.head)}
            </TableCell>
            <TableCell className="text-right tabular-nums">
              {formatPct(d.delta_pct)}
            </TableCell>
            <TableCell>
              {d.direction ? (
                <Badge variant={DIRECTION_VARIANT[d.direction]}>
                  {t(`benchmarks.compare.direction.${d.direction}`)}
                </Badge>
              ) : (
                <span className="text-xs text-muted-foreground">
                  {t('benchmarks.compare.oneSided')}
                </span>
              )}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}
