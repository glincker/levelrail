import { useTranslation } from 'react-i18next'
import { ChartColumnGlassIcon, DatabaseGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { EmptyState } from '@/components/ui/empty-state'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatBytes } from '@/lib/format'
import {
  useDatabaseEngineStats,
  type DatabaseEngineStats,
} from '../../queries/databaseEngineStats'

// Longer object lists are cut here so the table never needs virtualizing.
const MAX_OBJECTS = 50

const OBJECT_COUNT_KEYS = {
  meilisearch: 'viewer.engineStats.colDocuments',
  typesense: 'viewer.engineStats.colDocuments',
  qdrant: 'viewer.engineStats.colPoints',
} as const

function isObjectEngine(
  engine: string,
): engine is keyof typeof OBJECT_COUNT_KEYS {
  return engine in OBJECT_COUNT_KEYS
}

function ObjectsTable({ stats }: { stats: DatabaseEngineStats }) {
  const { t } = useTranslation('databases')
  if (stats.objects.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">
        {t('viewer.engineStats.emptyObjects')}
      </p>
    )
  }
  const sorted = [...stats.objects].sort((a, b) => b.count - a.count)
  const shown = sorted.slice(0, MAX_OBJECTS)
  const countLabel = isObjectEngine(stats.engine)
    ? t(OBJECT_COUNT_KEYS[stats.engine])
    : t('viewer.engineStats.colDocuments')
  return (
    <div className="space-y-2">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('viewer.engineStats.colName')}</TableHead>
            <TableHead className="text-right">{countLabel}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {shown.map((o) => (
            <TableRow key={o.name}>
              <TableCell className="font-mono text-sm">{o.name}</TableCell>
              <TableCell className="text-right tabular-nums">
                {o.count.toLocaleString()}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
      {sorted.length > shown.length ? (
        <p className="text-xs text-muted-foreground">
          {t('viewer.engineStats.more', {
            count: sorted.length - shown.length,
          })}
        </p>
      ) : null}
      <p className="text-xs text-muted-foreground">
        {t('viewer.engineStats.total', { count: stats.total })}
      </p>
    </div>
  )
}

function CacheMetrics({ stats }: { stats: DatabaseEngineStats }) {
  const { t } = useTranslation('databases')
  const m = stats.metrics ?? {}
  const hits = m.get_hits ?? 0
  const lookups = hits + (m.get_misses ?? 0)
  const rows: { label: string; value: string }[] = [
    {
      label: t('viewer.engineStats.items'),
      value: (m.curr_items ?? 0).toLocaleString(),
    },
    {
      label: t('viewer.engineStats.memory'),
      value: t('viewer.engineStats.memoryValue', {
        used: formatBytes(m.bytes ?? 0),
        limit: formatBytes(m.limit_maxbytes ?? 0),
      }),
    },
    {
      label: t('viewer.engineStats.hitRate'),
      value:
        lookups > 0
          ? t('viewer.engineStats.hitRateValue', {
              percent: Math.round((hits / lookups) * 100),
            })
          : t('viewer.engineStats.noLookups'),
    },
    {
      label: t('viewer.engineStats.evictions'),
      value: (m.evictions ?? 0).toLocaleString(),
    },
    {
      label: t('viewer.engineStats.connections'),
      value: (m.curr_connections ?? 0).toLocaleString(),
    },
  ]
  return (
    <dl className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
      {rows.map((r) => (
        <div key={r.label} className="rounded-lg border border-border p-3">
          <dt className="text-xs text-muted-foreground">{r.label}</dt>
          <dd className="text-sm font-medium text-foreground tabular-nums">
            {r.value}
          </dd>
        </div>
      ))}
    </dl>
  )
}

// Engines without a table or key browser: say so plainly, then show the
// live counts the engine's own API reports.
export function EngineStatsPanel({
  databaseName,
  engine,
}: {
  databaseName: string
  engine: string
}) {
  const { t } = useTranslation('databases')
  const stats = useDatabaseEngineStats(databaseName)
  const isCache = engine === 'memcached'

  return (
    <div className="space-y-4">
      <EmptyState
        compact
        icon={<DatabaseGlassIcon className="size-5.5" />}
        title={t('viewer.engineStats.title', { engine })}
        description={
          isCache
            ? t('viewer.engineStats.descriptionCache')
            : t('viewer.engineStats.description', { engine })
        }
      />
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <ChartColumnGlassIcon className="size-4.5" aria-hidden="true" />
            {isCache
              ? t('viewer.engineStats.cacheTitle')
              : t('viewer.engineStats.objectsTitle')}
          </CardTitle>
        </CardHeader>
        <CardContent>
          {stats.isPending ? (
            <div
              className="space-y-2"
              aria-label={t('viewer.engineStats.loading')}
            >
              <Skeleton className="h-5 w-full" />
              <Skeleton className="h-5 w-2/3" />
            </div>
          ) : stats.isError ? (
            <div className="space-y-2">
              <p className="text-sm font-medium text-foreground">
                {t('viewer.engineStats.errorTitle')}
              </p>
              <p className="text-sm text-muted-foreground">
                {t('viewer.engineStats.errorBody')}
              </p>
              <Button
                size="sm"
                variant="outline"
                onClick={() => {
                  void stats.refetch()
                }}
              >
                {t('viewer.engineStats.retry')}
              </Button>
            </div>
          ) : isCache ? (
            <CacheMetrics stats={stats.data} />
          ) : (
            <ObjectsTable stats={stats.data} />
          )}
        </CardContent>
      </Card>
    </div>
  )
}
