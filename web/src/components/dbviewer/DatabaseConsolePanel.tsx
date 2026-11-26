import { useTranslation } from 'react-i18next'
import { useDatabase } from '../../queries/databases'
import { BetaBadge } from '../BetaBadge'
import { ExplorerStatusStrip } from './ExplorerStatusStrip'
import { SqlConsole } from './SqlConsole'
import { SQL_ENGINES } from './engines'
import { UnsupportedEngine } from './UnsupportedEngine'

export function DatabaseConsolePanel({
  databaseName,
  initialSql,
}: {
  databaseName: string
  initialSql?: string
}) {
  const { t } = useTranslation('databases')
  const { data: database } = useDatabase(databaseName)

  return (
    <div className="space-y-4">
      <h1 className="flex items-center gap-2 text-(length:--glinr-type-size-page) leading-(--glinr-type-leading-page) font-(weight:--glinr-type-heading-weight)">
        {t('viewer.console.title')}
        <BetaBadge />
      </h1>
      <ExplorerStatusStrip databaseName={databaseName} />
      {SQL_ENGINES.has(database.engine) ? (
        <SqlConsole databaseName={databaseName} initialSql={initialSql} />
      ) : (
        <UnsupportedEngine engine={database.engine} />
      )}
    </div>
  )
}
