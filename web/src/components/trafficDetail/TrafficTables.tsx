import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { cn } from '@/lib/utils'
import type {
  RouteKind,
  TrafficKindTotal,
  TrafficLabel,
  TrafficReplica,
  TrafficRoute,
  TrafficSample,
} from '../../types/trafficDetail'
import { formatMs, formatPercent, formatRate } from './format'

function Section({
  title,
  hint,
  children,
}: Readonly<{ title: string; hint: string; children: ReactNode }>) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>{title}</CardTitle>
        <p className="text-sm text-muted-foreground">{hint}</p>
      </CardHeader>
      <CardContent>{children}</CardContent>
    </Card>
  )
}

const NUM = 'text-right tabular-nums'

export function ReplicaTable({ rows }: Readonly<{ rows: TrafficReplica[] }>) {
  const { t } = useTranslation('trafficDetail')
  return (
    <Section title={t('replicas.title')} hint={t('replicas.hint')}>
      {rows.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t('replicas.empty')}</p>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('replicas.upstream')}</TableHead>
              <TableHead className="text-right">
                {t('replicas.requests')}
              </TableHead>
              <TableHead className="text-right">{t('replicas.rate')}</TableHead>
              <TableHead className="text-right">
                {t('replicas.error5xx')}
              </TableHead>
              <TableHead className="text-right">{t('replicas.p50')}</TableHead>
              <TableHead className="text-right">{t('replicas.p95')}</TableHead>
              <TableHead className="text-right">{t('replicas.p99')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.map((r) => (
              <TableRow key={r.upstream}>
                <TableCell className="font-mono text-xs">
                  {r.upstream}
                </TableCell>
                <TableCell className={NUM}>{r.requests}</TableCell>
                <TableCell className={NUM}>
                  {formatRate(r.rate_per_sec)}
                </TableCell>
                <TableCell className={NUM}>
                  {formatPercent(r.error_rate_5xx)}
                </TableCell>
                <TableCell className={NUM}>{formatMs(r.p50_ms)}</TableCell>
                <TableCell className={NUM}>{formatMs(r.p95_ms)}</TableCell>
                <TableCell className={NUM}>{formatMs(r.p99_ms)}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
    </Section>
  )
}

function RouteRows({ rows }: Readonly<{ rows: TrafficRoute[] }>) {
  const { t } = useTranslation('trafficDetail')
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>{t('routes.route')}</TableHead>
          <TableHead className="text-right">{t('routes.requests')}</TableHead>
          <TableHead className="text-right">{t('routes.avg')}</TableHead>
          <TableHead className="text-right">{t('routes.error5xx')}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {rows.map((r) => (
          <TableRow key={r.route}>
            <TableCell className="font-mono text-xs">{r.route}</TableCell>
            <TableCell className={NUM}>{r.requests}</TableCell>
            <TableCell className={NUM}>{formatMs(r.avg_ms)}</TableCell>
            <TableCell className={NUM}>
              {formatPercent(r.error_rate_5xx)}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}

export function SlowRouteTable({
  rows,
  top = false,
}: Readonly<{ rows: TrafficRoute[]; top?: boolean }>) {
  const { t } = useTranslation('trafficDetail')
  return (
    <Section
      title={t(top ? 'routes.topTitle' : 'routes.title')}
      hint={t('routes.hint')}
    >
      {rows.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          {t(top ? 'routes.topEmpty' : 'routes.empty')}
        </p>
      ) : (
        <RouteRows rows={rows} />
      )}
    </Section>
  )
}

const KINDS: RouteKind[] = ['page', 'api', 'asset']

/** TopRoutesCard lists the busiest routes per kind, pages first, so static files never bury the pages. */
export function TopRoutesCard({
  rows,
  totals,
}: Readonly<{
  rows: TrafficRoute[]
  totals?: Partial<Record<RouteKind, TrafficKindTotal>>
}>) {
  const { t } = useTranslation('trafficDetail')
  const [kind, setKind] = useState<RouteKind>('page')
  const shown = rows.filter((r) => (r.kind ?? 'page') === kind)
  return (
    <Section
      title={t(`routes.topTitleByKind.${kind}`)}
      hint={t(`routes.kindHint.${kind}`)}
    >
      <div
        role="tablist"
        aria-label={t('routes.kindTabs')}
        className="mb-3 flex flex-wrap gap-2"
      >
        {KINDS.map((k) => (
          <button
            key={k}
            type="button"
            role="tab"
            aria-selected={kind === k}
            onClick={() => {
              setKind(k)
            }}
            className={cn(
              'rounded-full border border-border px-3 py-1 text-xs text-muted-foreground transition-colors hover:text-foreground',
              kind === k && 'bg-muted text-foreground',
            )}
          >
            {t(`routes.kind.${k}`)}
            <span className="ml-1.5 tabular-nums">
              {totals?.[k]?.requests ?? 0}
            </span>
          </button>
        ))}
      </div>
      {shown.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t('routes.topEmpty')}</p>
      ) : (
        <RouteRows rows={shown} />
      )}
    </Section>
  )
}

// The API caps each audience list at 10 rows, so these never need virtualizing.
export function AudienceTable({
  kind,
  rows,
}: Readonly<{ kind: 'referrers' | 'countries'; rows: TrafficLabel[] }>) {
  const { t } = useTranslation('trafficDetail')
  return (
    <Section
      title={t(`audience.${kind}.title`)}
      hint={t(`audience.${kind}.hint`)}
    >
      {rows.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          {t(`audience.${kind}.empty`)}
        </p>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t(`audience.${kind}.label`)}</TableHead>
              <TableHead className="text-right">
                {t('audience.requests')}
              </TableHead>
              <TableHead className="text-right">
                {t('audience.share')}
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.map((r) => (
              <TableRow key={r.label}>
                <TableCell className="font-mono text-xs">{r.label}</TableCell>
                <TableCell className={NUM}>{r.requests}</TableCell>
                <TableCell className={NUM}>{formatPercent(r.share)}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
    </Section>
  )
}

// The API caps samples at 50 rows, so this list never needs virtualizing.
export function SampleList({ rows }: Readonly<{ rows: TrafficSample[] }>) {
  const { t } = useTranslation('trafficDetail')
  return (
    <Section title={t('samples.title')} hint={t('samples.hint')}>
      {rows.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t('samples.empty')}</p>
      ) : (
        <ul className="divide-y divide-border">
          {rows.map((s) => (
            <li
              key={`${s.time}-${s.request_id ?? ''}-${s.route}`}
              className="grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-x-4 gap-y-1 px-2 py-2 text-sm"
            >
              <Badge variant={s.status >= 500 ? 'destructive' : 'muted'}>
                {s.status}
              </Badge>
              <span className="min-w-0">
                <span className="block truncate font-medium text-foreground">
                  <span className="mr-2 text-muted-foreground">{s.method}</span>
                  {s.route}
                </span>
                <span className="block truncate font-mono text-xs text-muted-foreground">
                  {new Date(s.time).toLocaleTimeString()}
                  {s.upstream ? ` ${s.upstream}` : ''}
                  {s.request_id ? ` ${s.request_id}` : ''}
                </span>
              </span>
              <span className="tabular-nums text-muted-foreground">
                {formatMs(s.duration_ms)}
              </span>
            </li>
          ))}
        </ul>
      )}
    </Section>
  )
}
