import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { RefreshCwGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { classifyExplorerError } from '../../../lib/explorerError'
import { formatSize } from '../../../lib/format'
import { useDynamoTables } from '../../../queries/databaseDynamo'
import { ExplorerLoading, ExplorerReadError } from '../explorer/ExplorerStates'
import { DynamoItems } from './DynamoItems'

export default function DynamoExplorer({
  databaseName,
}: {
  databaseName: string
}) {
  const { t } = useTranslation('databases')
  const tables = useDynamoTables(databaseName)
  const [picked, setPicked] = useState<string | null>(null)

  if (tables.isPending) return <ExplorerLoading />
  if (tables.isError) {
    return (
      <ExplorerReadError
        info={classifyExplorerError(tables.error)}
        retrying={tables.isFetching}
        onRetry={() => void tables.refetch()}
      />
    )
  }
  const data = tables.data
  const selected = data.tables.find(
    (tb) => tb.TableName === (picked ?? data.tables[0]?.TableName),
  )

  return (
    <div className="grid gap-4 md:grid-cols-[15rem_minmax(0,1fr)]">
      <nav aria-label={t('viewer.dynamo.tables')} className="space-y-2">
        <div className="flex items-center justify-between gap-2">
          <h2 className="text-sm font-medium">{t('viewer.dynamo.tables')}</h2>
          <Button
            type="button"
            size="icon"
            variant="outline"
            disabled={tables.isFetching}
            title={t('viewer.explorer.refresh')}
            aria-label={t('viewer.explorer.refresh')}
            onClick={() => void tables.refetch()}
          >
            <RefreshCwGlassIcon className="size-5" aria-hidden="true" />
          </Button>
        </div>
        {data.truncated ? (
          <p className="rounded-md bg-tone-warning-soft px-2 py-1 text-xs text-tone-warning">
            {t('viewer.dynamo.truncated')}
          </p>
        ) : null}
        <ul className="max-h-[36rem] space-y-1 overflow-auto">
          {data.tables.map((tb) => (
            <li key={tb.TableName}>
              <button
                type="button"
                aria-current={
                  tb.TableName === selected?.TableName ? 'true' : undefined
                }
                onClick={() => {
                  setPicked(tb.TableName)
                }}
                className={cn(
                  'w-full rounded-md px-2 py-1 text-left text-sm hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring',
                  tb.TableName === selected?.TableName &&
                    'bg-muted font-medium',
                )}
              >
                <span className="block truncate font-mono">{tb.TableName}</span>
                <span className="block truncate text-xs font-normal text-muted-foreground">
                  {t('viewer.dynamo.meta', {
                    count: tb.ItemCount ?? 0,
                    size: formatSize(tb.TableSizeBytes ?? 0),
                  })}
                </span>
              </button>
            </li>
          ))}
        </ul>
        {data.tables.length > 0 ? (
          <p className="text-xs text-muted-foreground">
            {t('viewer.dynamo.estimate')}
          </p>
        ) : null}
      </nav>
      {selected ? (
        <DynamoItems
          key={selected.TableName}
          name={databaseName}
          table={selected}
        />
      ) : (
        <p className="rounded-lg border border-dashed border-border p-6 text-center text-sm text-muted-foreground">
          {t('viewer.dynamo.noTables')}
        </p>
      )}
    </div>
  )
}
