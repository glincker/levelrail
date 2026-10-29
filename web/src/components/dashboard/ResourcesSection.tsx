import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  BoxGlassIcon,
  DatabaseGlassIcon,
  PackageGlassIcon,
} from '@thesvg/ui-react'
import { Progress as ProgressPrimitive } from '@base-ui/react/progress'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { SkeletonTile, Sparkline } from '@/components/kit'
import { TONE, type Tone } from '../kit/tone'
import { cn } from '@/lib/utils'
import { formatSize } from '../../lib/format'
import { useBrand } from '../../hooks/useBrand'
import {
  rankConsumers,
  ratio,
  type Consumer,
  type ConsumerSort,
  type HostGauge,
  type MemoryBreakdown,
} from '../../lib/fleetResources'
import {
  useDashboardResources,
  type DashboardResources,
} from '../../queries/dashboardResources'
import { allocateWidths, widthClass } from './widthClass'
import { FLOATING_CARD } from './cardStyle'
import { InstanceWideLabel } from './ScopeIndicator'

const TOP_ROWS = 8
const TABLE_CAP = 50
const WARN_RATIO = 0.8
const CRIT_RATIO = 0.92

function gaugeTone(r: number): Tone {
  if (r >= CRIT_RATIO) return 'danger'
  if (r >= WARN_RATIO) return 'warning'
  return 'accent'
}

function Bar({
  value,
  tone,
  label,
}: {
  value: number
  tone: Tone
  label: string
}) {
  return (
    <ProgressPrimitive.Root value={Math.round(value * 100)} aria-label={label}>
      <ProgressPrimitive.Track className="h-1.5 w-full overflow-hidden rounded-full bg-muted">
        <ProgressPrimitive.Indicator
          className={cn('h-full transition-[width]', TONE[tone].solid)}
        />
      </ProgressPrimitive.Track>
    </ProgressPrimitive.Root>
  )
}

function Gauge({
  label,
  value,
  detail,
}: {
  label: string
  value: number
  detail: string
}) {
  return (
    <div className="space-y-1">
      <div className="flex items-baseline justify-between gap-2 text-xs">
        <span className="text-muted-foreground">{label}</span>
        <span className="tabular-nums text-foreground">{detail}</span>
      </div>
      <Bar value={value} tone={gaugeTone(value)} label={label} />
    </div>
  )
}

function HostCard({ host }: { host: HostGauge }) {
  const { t } = useTranslation('dashboard')
  const cpuRatio = host.cpu?.cores
    ? ratio(host.cpu.percent / 100, host.cpu.cores)
    : undefined
  return (
    <div className={cn('space-y-3 p-4', FLOATING_CARD)}>
      <div className="flex items-center justify-between gap-2">
        <h3 className="truncate text-(length:--glinr-type-size-section) leading-(--glinr-type-leading-section) font-(weight:--glinr-type-heading-weight)">
          {host.name}
        </h3>
        {host.isLocal ? (
          <span className="text-xs text-muted-foreground">
            {t('resources.controlPlane')}
          </span>
        ) : null}
      </div>
      {host.cpu ? (
        cpuRatio !== undefined ? (
          <Gauge
            label={t('resources.cpu')}
            value={cpuRatio}
            detail={t('resources.cpuOfCores', {
              percent: host.cpu.percent.toFixed(1),
              cores: host.cpu.cores,
            })}
          />
        ) : (
          <div className="flex items-baseline justify-between text-xs">
            <span className="text-muted-foreground">{t('resources.cpu')}</span>
            <span className="tabular-nums">
              {t('resources.cpuContainers', {
                percent: host.cpu.percent.toFixed(1),
              })}
            </span>
          </div>
        )
      ) : null}
      {host.memory ? (
        <Gauge
          label={t('resources.memory')}
          value={ratio(host.memory.usedBytes, host.memory.totalBytes)}
          detail={`${formatSize(host.memory.usedBytes)} / ${formatSize(host.memory.totalBytes)}`}
        />
      ) : null}
      {host.disk ? (
        <Gauge
          label={t('resources.disk')}
          value={ratio(host.disk.usedBytes, host.disk.totalBytes)}
          detail={`${formatSize(host.disk.usedBytes)} / ${formatSize(host.disk.totalBytes)}`}
        />
      ) : null}
      {!host.cpu && !host.memory && !host.disk ? (
        <p className="text-xs text-muted-foreground">
          {t('resources.noReadings')}
        </p>
      ) : null}
    </div>
  )
}

interface Segment {
  key: 'levelrail' | 'apps' | 'databases' | 'other' | 'system' | 'free'
  bytes: number
  className: string
}

function MemoryStack({ breakdown }: { breakdown: MemoryBreakdown }) {
  const { t } = useTranslation('dashboard')
  const brand = useBrand().Name
  const showAttribution =
    breakdown.systemKnown ||
    breakdown.levelrail > 0 ||
    breakdown.otherContainers > 0
  const segments: Segment[] = [
    ...(showAttribution
      ? [
          {
            key: 'levelrail' as const,
            bytes: breakdown.levelrail,
            className: TONE.success.solid,
          },
        ]
      : []),
    { key: 'apps', bytes: breakdown.apps, className: TONE.accent.solid },
    {
      key: 'databases',
      bytes: breakdown.databases,
      className: TONE.info.solid,
    },
    ...(showAttribution
      ? [
          {
            key: 'other' as const,
            bytes: breakdown.otherContainers,
            className: TONE.warning.solid,
          },
        ]
      : []),
    ...(breakdown.systemKnown
      ? [
          {
            key: 'system' as const,
            bytes: breakdown.system,
            className: TONE.neutral.solid,
          },
        ]
      : []),
    { key: 'free', bytes: breakdown.free, className: 'bg-muted' },
  ]
  const widths = allocateWidths(
    segments.map((s) => s.bytes),
    breakdown.totalBytes,
  )
  return (
    <div className="space-y-2">
      <div className="flex items-baseline justify-between gap-2">
        <h3 className="text-(length:--glinr-type-size-section) leading-(--glinr-type-leading-section) font-(weight:--glinr-type-heading-weight)">
          {t('resources.memoryBreakdown')}
        </h3>
        <span className="text-xs tabular-nums text-muted-foreground">
          {formatSize(breakdown.totalBytes)}
        </span>
      </div>
      <div
        role="img"
        aria-label={t('resources.memoryBreakdown')}
        className="flex h-2.5 w-full overflow-hidden rounded-full bg-muted"
      >
        {segments.map((s, i) => (
          <span
            key={s.key}
            className={cn('h-full', s.className, widthClass(widths[i] ?? 0))}
          />
        ))}
      </div>
      <ul className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
        {segments.map((s) => (
          <li
            key={s.key}
            title={t(`resources.segmentHelp.${s.key}`, { brand })}
            className="flex items-center gap-1.5"
          >
            <span
              aria-hidden="true"
              className={cn('size-2 rounded-full', s.className)}
            />
            {t(`resources.segment.${s.key}`, { brand })}
            <span className="tabular-nums text-foreground">
              {formatSize(s.bytes)}
            </span>
          </li>
        ))}
      </ul>
    </div>
  )
}

function ConsumerRow({
  c,
  max,
  sort,
}: {
  c: Consumer
  max: number
  sort: ConsumerSort
}) {
  const { t } = useTranslation('dashboard')
  const brand = useBrand().Name
  const value = sort === 'memory' ? c.memoryBytes : c.cpuPercent
  const Icon =
    c.kind === 'app'
      ? PackageGlassIcon
      : c.kind === 'database'
        ? DatabaseGlassIcon
        : BoxGlassIcon
  const content = (
    <>
      <Icon
        aria-label={t(`resources.kind.${c.kind}`)}
        className="size-4 shrink-0 text-muted-foreground"
      />
      <span className="min-w-0 flex-1 truncate text-sm font-medium">
        {c.name}
      </span>
      {c.kind === 'other' ? (
        <Badge variant="warning">{t('resources.notManaged', { brand })}</Badge>
      ) : null}
      <span className="hidden w-16 shrink-0 sm:block">
        {c.spark && c.spark.some((v) => v > 0) ? (
          <Sparkline
            values={c.spark}
            tone="neutral"
            width={64}
            height={18}
            ariaLabel={t('resources.requestTrend', { name: c.name })}
          />
        ) : null}
      </span>
      <span className="w-20 shrink-0 sm:w-28">
        <Bar
          value={ratio(value, max)}
          tone={
            c.kind === 'app'
              ? 'accent'
              : c.kind === 'database'
                ? 'info'
                : 'warning'
          }
          label={c.name}
        />
      </span>
      <span className="w-16 shrink-0 text-right text-xs tabular-nums text-muted-foreground">
        {sort === 'memory'
          ? formatSize(c.memoryBytes)
          : `${c.cpuPercent.toFixed(1)}%`}
      </span>
    </>
  )
  const cls =
    'flex items-center gap-3 px-3 py-2 hover:bg-muted/40 focus-visible:ring-2 focus-visible:ring-ring/60 focus-visible:outline-none'
  if (c.kind === 'other') {
    return (
      <Link to="/settings/containers" className={cls}>
        {content}
      </Link>
    )
  }
  return c.kind === 'app' ? (
    <Link to="/apps/$name" params={{ name: c.name }} className={cls}>
      {content}
    </Link>
  ) : (
    <Link to="/databases/$name" params={{ name: c.name }} className={cls}>
      {content}
    </Link>
  )
}

export function TopConsumers({ consumers }: { consumers: Consumer[] }) {
  const { t } = useTranslation('dashboard')
  const [sort, setSort] = useState<ConsumerSort>('memory')
  const [all, setAll] = useState(false)
  const ranked = rankConsumers(consumers, sort)
  if (ranked.length === 0) return null
  const shown = all ? ranked.slice(0, TABLE_CAP) : ranked.slice(0, TOP_ROWS)
  const first = ranked[0]
  const max = first
    ? sort === 'memory'
      ? first.memoryBytes
      : first.cpuPercent
    : 0
  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between gap-2">
        <h3 className="text-(length:--glinr-type-size-section) leading-(--glinr-type-leading-section) font-(weight:--glinr-type-heading-weight)">
          {t('resources.topConsumers')}
        </h3>
        <div
          role="group"
          aria-label={t('resources.sortBy')}
          className="flex gap-1"
        >
          {(['memory', 'cpu'] as const).map((s) => (
            <Button
              key={s}
              type="button"
              size="sm"
              variant={sort === s ? 'secondary' : 'ghost'}
              aria-pressed={sort === s}
              className="h-6 px-2 text-xs"
              onClick={() => {
                setSort(s)
              }}
            >
              {t(`resources.sort.${s}`)}
            </Button>
          ))}
        </div>
      </div>
      <ul
        className={cn('divide-y divide-border overflow-hidden', FLOATING_CARD)}
      >
        {shown.map((c) => (
          <li key={`${c.kind}:${c.name}`}>
            <ConsumerRow c={c} max={max} sort={sort} />
          </li>
        ))}
      </ul>
      {ranked.length > TOP_ROWS ? (
        <Button
          type="button"
          size="sm"
          variant="ghost"
          onClick={() => {
            setAll((v) => !v)
          }}
        >
          {all
            ? t('resources.showLess')
            : t('resources.viewAll', { count: ranked.length })}
        </Button>
      ) : null}
    </div>
  )
}

export function ResourcesView({ data }: { data: DashboardResources }) {
  const { t } = useTranslation('dashboard')
  if (data.isPending) {
    return (
      <section aria-label={t('resources.title')} aria-busy="true">
        <SkeletonTile className="h-48" />
      </section>
    )
  }
  const empty =
    data.gauges.length === 0 &&
    data.consumers.length === 0 &&
    data.breakdown === undefined
  if (empty) return null
  return (
    <section aria-label={t('resources.title')} className="space-y-4">
      <div className="flex items-baseline justify-between gap-2">
        <h2 className="text-(length:--glinr-type-size-section) leading-(--glinr-type-leading-section) font-(weight:--glinr-type-heading-weight) text-foreground">
          {t('resources.title')}
        </h2>
        {data.databaseCount > 0 ? (
          <Link
            to="/databases"
            className="text-xs text-muted-foreground hover:text-foreground"
          >
            {t('resources.databasesLine', { count: data.databaseCount })}
          </Link>
        ) : null}
      </div>
      {data.gauges.length > 0 ? (
        <div className="space-y-2">
          <InstanceWideLabel />
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3 @4xl:grid-cols-4">
            {data.gauges.map((g) => (
              <HostCard key={g.id} host={g} />
            ))}
          </div>
        </div>
      ) : null}
      {data.breakdown ? <MemoryStack breakdown={data.breakdown} /> : null}
      <TopConsumers consumers={data.consumers} />
    </section>
  )
}

export function ResourcesSection() {
  return <ResourcesView data={useDashboardResources()} />
}
