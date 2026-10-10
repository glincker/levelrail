import { useTranslation } from 'react-i18next'
import { useDatabase } from '../../queries/databases'
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
      <h1 className="text-lg font-semibold">{t('viewer.console.title')}</h1>
      {SQL_ENGINES.has(database.engine) ? (
        <SqlConsole databaseName={databaseName} initialSql={initialSql} />
      ) : (
        <UnsupportedEngine engine={database.engine} />
      )}
    </div>
  )
}
