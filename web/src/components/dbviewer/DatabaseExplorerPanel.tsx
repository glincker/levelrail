import { useTranslation } from 'react-i18next'
import { useDatabase } from '../../queries/databases'
import { BetaBadge } from '../BetaBadge'
import { RedisKeyBrowser } from './RedisKeyBrowser'
import { SchemaExplorer } from './SchemaExplorer'
import { KV_ENGINES, SQL_ENGINES } from './engines'
import { UnsupportedEngine } from './UnsupportedEngine'

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
      <h1 className="flex items-center gap-2 text-lg font-semibold">
        {KV_ENGINES.has(engine)
          ? t('viewer.keys.title')
          : t('viewer.explorer.title')}
        <BetaBadge />
      </h1>
      {SQL_ENGINES.has(engine) ? (
        <SchemaExplorer databaseName={databaseName} />
      ) : KV_ENGINES.has(engine) ? (
        <RedisKeyBrowser databaseName={databaseName} />
      ) : (
        <UnsupportedEngine engine={engine} />
      )}
    </div>
  )
}
