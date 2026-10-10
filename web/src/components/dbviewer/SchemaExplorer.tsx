import { useMemo, useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  useDatabaseSchema,
  useDatabaseTableStructure,
} from '../../queries/databaseViewer'
import { formatSize } from '../../lib/format'
import { dialectOf, selectStatement } from '../../lib/explorerData'
import {
  formatRowEstimate,
  readLastTable,
  tableKey,
  writeLastTable,
} from '../../lib/explorerTree'
import type { DbSchemaNode, DbTable } from '../../types/databaseViewer'
import { useDatabaseDataCopy } from '../../queries/databaseDataCopy'
import { classifyExplorerError } from '../../lib/explorerError'
import {
  ExplorerEmpty,
  ExplorerLoading,
  ExplorerReadError,
} from './explorer/ExplorerStates'
import { StructureView } from './explorer/StructureView'
import { TableTree } from './explorer/TableTree'
import { TableDataView } from './TableDataView'

const CONSOLE_PREVIEW_LIMIT = 100

interface Selected {
  schema: string
  table: string
}

function findTable(
  schemas: DbSchemaNode[] | undefined,
  sel: Selected | null,
): DbTable | undefined {
  if (!sel) return undefined
  return schemas
    ?.find((s) => s.name === sel.schema)
    ?.tables.find((tb) => tb.name === sel.table)
}

function parseKey(
  schemas: DbSchemaNode[],
  key: string | null,
): Selected | null {
  if (!key) return null
  for (const s of schemas) {
    for (const tb of s.tables) {
      if (tableKey(s.name, tb.name) === key) {
        return { schema: s.name, table: tb.name }
      }
    }
  }
  return null
}

// Schema browser plus table viewer for Postgres, MySQL, and MariaDB. The
// schema call returns an outline only; columns, keys and DDL load when a
// table is opened.
export function SchemaExplorer({ databaseName }: { databaseName: string }) {
  const { t } = useTranslation('databases')
  const navigate = useNavigate()
  const { data, isLoading, error, refetch, isFetching } = useDatabaseSchema(
    databaseName,
    true,
  )
  const copy = useDatabaseDataCopy(databaseName)
  const [picked, setPicked] = useState<Selected | null>(null)
  const [tab, setTab] = useState('data')

  const selected = useMemo((): Selected | null => {
    if (picked) return picked
    if (!data) return null
    const last = parseKey(data.schemas, readLastTable(databaseName))
    if (last) return last
    const first = data.schemas[0]
    const firstTable = first?.tables[0]
    return first && firstTable
      ? { schema: first.name, table: firstTable.name }
      : null
  }, [picked, data, databaseName])

  const table = useMemo(
    () => findTable(data?.schemas, selected),
    [data, selected],
  )
  const structure = useDatabaseTableStructure(
    databaseName,
    selected?.schema ?? '',
    selected?.table ?? '',
    selected !== null && table !== undefined,
  )

  function open(schema: string, name: string) {
    setPicked({ schema, table: name })
    writeLastTable(databaseName, tableKey(schema, name))
  }

  function openConsole() {
    if (!selected || !data) return
    void navigate({
      to: '/databases/$name/console',
      params: { name: databaseName },
      search: {
        sql: selectStatement(
          dialectOf(data.engine),
          selected.schema,
          selected.table,
          CONSOLE_PREVIEW_LIMIT,
        ),
      },
    })
  }

  if (isLoading) return <ExplorerLoading />
  if (error) {
    return (
      <ExplorerReadError
        info={classifyExplorerError(error)}
        retrying={isFetching}
        onRetry={() => {
          void refetch()
        }}
      />
    )
  }
  if (!data) return <ExplorerLoading />
  if (data.schemas.length === 0) {
    return (
      <ExplorerEmpty
        databaseName={databaseName}
        evidence={data.evidence}
        checkedAt={data.checked_at}
        copy={copy.data}
      />
    )
  }

  return (
    <div className="grid gap-4 lg:grid-cols-[20rem_minmax(0,1fr)]">
      <TableTree
        schemas={data.schemas}
        selectedKey={
          selected ? tableKey(selected.schema, selected.table) : null
        }
        onSelect={open}
        onRefresh={() => {
          void refetch()
        }}
        refreshing={isFetching}
        truncatedAt={data.truncated ? data.table_limit : undefined}
      />

      <section className="min-w-0">
        {selected && table ? (
          <div className="space-y-3">
            <div>
              <h2 className="text-base font-semibold">
                {selected.schema}.{table.name}
              </h2>
              <p className="text-xs text-muted-foreground">
                {table.kind} |{' '}
                {t('viewer.explorer.estimate', {
                  count: table.row_estimate,
                  short: formatRowEstimate(table.row_estimate),
                })}{' '}
                | {formatSize(table.size_bytes)}
              </p>
            </div>
            <Tabs value={tab} onValueChange={setTab}>
              <TabsList>
                <TabsTrigger value="data">
                  {t('viewer.explorer.tabData')}
                </TabsTrigger>
                <TabsTrigger value="structure">
                  {t('viewer.explorer.tabStructure')}
                </TabsTrigger>
              </TabsList>
              <TabsContent value="data">
                <TableDataView
                  key={tableKey(selected.schema, table.name)}
                  databaseName={databaseName}
                  engine={data.engine}
                  schema={selected.schema}
                  table={table.name}
                  columns={structure.data?.columns}
                  limits={data.limits}
                />
              </TabsContent>
              <TabsContent value="structure">
                <StructureView
                  structure={structure.data}
                  loading={structure.isLoading}
                  error={structure.error}
                  onOpenTable={(schema, name) => {
                    open(schema, name)
                  }}
                  onOpenConsole={openConsole}
                />
              </TabsContent>
            </Tabs>
          </div>
        ) : (
          <p className="rounded-lg border border-dashed border-border p-8 text-center text-sm text-muted-foreground">
            {t('viewer.explorer.selectTable')}
          </p>
        )}
      </section>
    </div>
  )
}
