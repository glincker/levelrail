import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { classifyExplorerError } from '../../../lib/explorerError'
import { useMongoCollections } from '../../../queries/databaseMongo'
import { useExplorerCapabilities } from '../../../queries/databaseViewer'
import { ExplorerLoading, ExplorerReadError } from '../explorer/ExplorerStates'
import { MongoDocuments } from './MongoDocuments'
import { MongoIndexes } from './MongoIndexes'
import { MongoStats } from './MongoStats'
import { MongoTree, type MongoSelection } from './MongoTree'

export default function MongoExplorer({
  databaseName,
}: {
  databaseName: string
}) {
  const { t } = useTranslation('databases')
  const cols = useMongoCollections(databaseName)
  const caps = useExplorerCapabilities(databaseName)
  const [picked, setPicked] = useState<MongoSelection | null>(null)

  if (cols.isPending) return <ExplorerLoading />
  if (cols.isError) {
    return (
      <ExplorerReadError
        info={classifyExplorerError(cols.error)}
        retrying={cols.isFetching}
        onRetry={() => void cols.refetch()}
      />
    )
  }
  const data = cols.data
  const first = data.collections[0]
  const selected =
    picked ?? (first ? { database: first.db, collection: first.name } : null)
  const info = selected
    ? data.collections.find(
        (c) => c.db === selected.database && c.name === selected.collection,
      )
    : undefined

  return (
    <div className="grid gap-4 md:grid-cols-[15rem_minmax(0,1fr)]">
      <MongoTree
        data={data}
        selected={selected}
        onSelect={setPicked}
        onRefresh={() => void cols.refetch()}
        refreshing={cols.isFetching}
      />
      {selected && info ? (
        <div className="min-w-0 space-y-2">
          <h2 className="truncate font-mono text-sm font-medium">
            {selected.database}.{selected.collection}
          </h2>
          <Tabs defaultValue="documents">
            <TabsList>
              <TabsTrigger value="documents">
                {t('viewer.mongo.tabs.documents')}
              </TabsTrigger>
              <TabsTrigger value="indexes">
                {t('viewer.mongo.tabs.indexes')}
              </TabsTrigger>
              <TabsTrigger value="stats">
                {t('viewer.mongo.tabs.stats')}
              </TabsTrigger>
            </TabsList>
            <TabsContent value="documents">
              <MongoDocuments
                key={`${selected.database}/${selected.collection}`}
                name={databaseName}
                database={selected.database}
                collection={selected.collection}
                maxRows={caps.data?.limits.max_rows ?? 0}
              />
            </TabsContent>
            <TabsContent value="indexes">
              <MongoIndexes info={info} />
            </TabsContent>
            <TabsContent value="stats">
              <MongoStats info={info} />
            </TabsContent>
          </Tabs>
        </div>
      ) : (
        <p className="rounded-lg border border-dashed border-border p-6 text-center text-sm text-muted-foreground">
          {t('viewer.mongo.noCollections')}
        </p>
      )}
    </div>
  )
}
