import { lazy, Suspense } from 'react'
import { useTranslation } from 'react-i18next'
import { Skeleton } from '@/components/ui/skeleton'
import { useDatabase } from '../../queries/databases'
import { BetaBadge } from '../BetaBadge'
import { SchemaExplorer } from './SchemaExplorer'
import { EngineStatsPanel } from './EngineStatsPanel'
import { KV_ENGINES, SQL_ENGINES, STATS_ENGINES } from './engines'
import { UnsupportedEngine } from './UnsupportedEngine'

const RedisKeyBrowser = lazy(() =>
  import('./RedisKeyBrowser').then((m) => ({ default: m.RedisKeyBrowser })),
)

// Explorer tab: schema browser and table data for SQL engines, key
// browser for Redis-compatible engines.
export function DatabaseExplorerPanel({
  databaseName,
}: {
  databaseName: string
}) {
  const { t } = useTranslation('databases')
  const { data: database } = useDatabase(databaseName)
  const engine = database.engine

  return (
    <div className="space-y-4">
      <h1 className="flex items-center gap-2 text-(length:--glinr-type-size-page) leading-(--glinr-type-leading-page) font-(weight:--glinr-type-heading-weight)">
        {KV_ENGINES.has(engine)
          ? t('viewer.keys.title')
          : t('viewer.explorer.title')}
        <BetaBadge />
      </h1>
      {SQL_ENGINES.has(engine) ? (
        <SchemaExplorer databaseName={databaseName} />
      ) : KV_ENGINES.has(engine) ? (
        <Suspense fallback={<Skeleton className="h-48 w-full" />}>
          <RedisKeyBrowser databaseName={databaseName} />
        </Suspense>
      ) : STATS_ENGINES.has(engine) ? (
        <EngineStatsPanel databaseName={databaseName} engine={engine} />
      ) : (
        <UnsupportedEngine engine={engine} />
      )}
    </div>
  )
}
