import { useMemo, useState } from 'react'
import {
  ArrowDownGlassIcon,
  ArrowUpDownGlassIcon,
  ArrowUpGlassIcon,
} from '@thesvg/ui-react'
import { useTranslation } from 'react-i18next'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { RATING_TONE, rateVital } from '../../lib/vitalsThresholds'
import {
  formatVitalValue,
  type AppVitals,
  type VitalsMetric,
  type VitalsRoute,
  VITALS_METRICS,
} from '../../queries/appVitals'
import { SnippetBlock } from './VitalsSnippet'

type SortKey = 'route' | 'samples' | VitalsMetric
interface Sort {
  key: SortKey
  dir: 'asc' | 'desc'
}

function samples(r: VitalsRoute): number {
  return Math.max(0, ...Object.values(r.metrics).map((s) => s?.count ?? 0))
}

// Missing values always sink to the bottom regardless of direction.
function sortRoutes(routes: VitalsRoute[], sort: Sort): VitalsRoute[] {
  const sign = sort.dir === 'asc' ? 1 : -1
  const val = (r: VitalsRoute): number | null => {
    if (sort.key === 'samples') return samples(r)
    if (sort.key === 'route') return null
    const s = r.metrics[sort.key]
    return s && s.count > 0 ? s.p75 : null
  }
  return [...routes].sort((a, b) => {
    if (sort.key === 'route') return sign * a.route.localeCompare(b.route)
    const va = val(a)
    const vb = val(b)
    if (va === null && vb === null) return a.route.localeCompare(b.route)
    if (va === null) return 1
    if (vb === null) return -1
    return sign * (va - vb) || a.route.localeCompare(b.route)
  })
}

function Cell({ metric, route }: { metric: VitalsMetric; route: VitalsRoute }) {
  const stat = route.metrics[metric]
  if (!stat || stat.count === 0) {
    return <span className="text-muted-foreground">-</span>
  }
  const text = stat.overflow
    ? `>${metric === 'CLS' ? stat.p75 : `${stat.p75 / 1000} s`}`
    : formatVitalValue(metric, stat.p75)
  return (
    <span className={RATING_TONE[rateVital(metric, stat.p75)]}>{text}</span>
  )
}

interface Props {
  appName: string
  data: AppVitals
  open: boolean
  onOpenChange: (open: boolean) => void
}

/** VitalsRoutesDialog lists every route with sortable p75 values; loaded only when opened. */
export default function VitalsRoutesDialog({
  appName,
  data,
  open,
  onOpenChange,
}: Props) {
  const { t } = useTranslation('webVitals')
  const [sort, setSort] = useState<Sort>({ key: 'samples', dir: 'desc' })
  const rows = useMemo(() => sortRoutes(data.routes, sort), [data, sort])

  const toggle = (key: SortKey) =>
    setSort((s) =>
      s.key === key
        ? { key, dir: s.dir === 'asc' ? 'desc' : 'asc' }
        : { key, dir: key === 'route' ? 'asc' : 'desc' },
    )

  const header = (key: SortKey, label: string) => {
    const active = sort.key === key
    const Icon = !active
      ? ArrowUpDownGlassIcon
      : sort.dir === 'asc'
        ? ArrowUpGlassIcon
        : ArrowDownGlassIcon
    return (
      <TableHead
        key={key}
        aria-sort={
          active ? (sort.dir === 'asc' ? 'ascending' : 'descending') : 'none'
        }
      >
        <button
          type="button"
          className="inline-flex items-center gap-1 rounded font-medium hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
          onClick={() => toggle(key)}
        >
          {label}
          <Icon size="1em" aria-hidden="true" />
        </button>
      </TableHead>
    )
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex max-h-[90dvh] min-w-0 flex-col gap-3 overflow-hidden sm:max-h-[90dvh] sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>{t('routesTitle')}</DialogTitle>
          <DialogDescription>
            {t('windowNote', { days: data.days })}
          </DialogDescription>
        </DialogHeader>
        {rows.length === 0 ? (
          <div className="space-y-2">
            <p className="text-sm text-muted-foreground">
              {data.enabled ? t('emptyEnabled') : t('emptyDisabled')}
            </p>
            {data.public_key ? (
              <SnippetBlock app={appName} publicKey={data.public_key} />
            ) : null}
          </div>
        ) : (
          <div className="min-h-0 overflow-auto">
            <Table aria-label={t('a11y.tables.vitalsRoutes', { ns: 'common' })}>
              <TableHeader>
                <TableRow>
                  {header('route', t('route'))}
                  {VITALS_METRICS.map((m) => header(m, m))}
                  {header('samples', t('samples'))}
                </TableRow>
              </TableHeader>
              <TableBody>
                {rows.map((r) => (
                  <TableRow key={r.route}>
                    <TableCell className="font-mono text-xs">
                      {r.route}
                    </TableCell>
                    {VITALS_METRICS.map((m) => (
                      <TableCell key={m}>
                        <Cell metric={m} route={r} />
                      </TableCell>
                    ))}
                    <TableCell>{samples(r)}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}
        <p className="text-xs text-muted-foreground">{t('legend')}</p>
      </DialogContent>
    </Dialog>
  )
}
