import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ArrowsClockwiseIcon,
  KeyIcon,
  TableIcon,
  EyeIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { EmptyState } from '@/components/ui/empty-state'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { cn } from '@/lib/utils'
import { useDatabaseSchema } from '../../queries/databaseViewer'
import type { DbSchemaNode, DbTable } from '../../types/databaseViewer'
import { TableDataView } from './TableDataView'

const KIB = 1024

function formatBytes(n: number): string {
  if (n < KIB) {
    return `${n} B`
  }
  const units = ['KiB', 'MiB', 'GiB', 'TiB']
  let value = n / KIB
  let i = 0
  while (value >= KIB && i < units.length - 1) {
    value /= KIB
    i += 1
  }
  return `${value.toFixed(1)} ${units[i] ?? ''}`
}

interface Selected {
  schema: string
  table: string
}

// Schema browser plus table viewer for Postgres, MySQL, and MariaDB.
export function SchemaExplorer({ databaseName }: { databaseName: string }) {
  const { t } = useTranslation('databases')
  const { data, isLoading, error, refetch, isFetching } = useDatabaseSchema(
    databaseName,
    true,
  )
  const [selected, setSelected] = useState<Selected | null>(null)
  const [filter, setFilter] = useState('')

  const groups = useMemo(() => {
    const needle = filter.trim().toLowerCase()
    return (data?.schemas ?? [])
      .map((s): DbSchemaNode => ({
        ...s,
        tables: s.tables.filter((tb) => tb.name.toLowerCase().includes(needle)),
      }))
      .filter((s) => s.tables.length > 0)
  }, [data, filter])

  const current = useMemo((): { schema: string; table: DbTable } | null => {
    if (!selected) {
      return null
    }
    const table = data?.schemas
      .find((s) => s.name === selected.schema)
      ?.tables.find((tb) => tb.name === selected.table)
    return table ? { schema: selected.schema, table } : null
  }, [data, selected])

  if (isLoading) {
    return (
      <Skeleton
        className="h-64 w-full"
        aria-label={t('viewer.explorer.loading')}
      />
    )
  }
  if (error) {
    return (
      <p className="text-sm text-destructive" role="alert">
        {error.message}
      </p>
    )
  }
  if (!data || data.schemas.length === 0) {
    return (
      <EmptyState
        icon={<TableIcon className="size-5" />}
        title={t('viewer.explorer.emptyTitle')}
        description={t('viewer.explorer.empty')}
      />
    )
  }

  return (
    <div className="grid gap-4 lg:grid-cols-[18rem_minmax(0,1fr)]">
      <aside className="space-y-2">
        <div className="flex items-center gap-2">
          <Input
            value={filter}
            placeholder={t('viewer.explorer.searchTables')}
            aria-label={t('viewer.explorer.searchTables')}
            onChange={(e) => {
              setFilter(e.target.value)
            }}
          />
          <Button
            type="button"
            size="icon"
            variant="outline"
            disabled={isFetching}
            title={t('viewer.explorer.refresh')}
            aria-label={t('viewer.explorer.refresh')}
            onClick={() => {
              void refetch()
            }}
          >
            <ArrowsClockwiseIcon className="size-4" aria-hidden="true" />
          </Button>
        </div>
        <nav
          aria-label={t('viewer.explorer.tables')}
          className="max-h-[32rem] space-y-3 overflow-auto rounded-lg border border-border p-2"
        >
          {groups.map((s) => (
            <div key={s.name}>
              <p className="px-2 pb-1 text-xs font-medium uppercase text-muted-foreground">
                {s.name}
              </p>
              <ul>
                {s.tables.map((tb) => {
                  const active =
                    selected?.schema === s.name && selected.table === tb.name
                  return (
                    <li key={tb.name}>
                      <button
                        type="button"
                        aria-current={active ? 'true' : undefined}
                        className={cn(
                          'flex w-full items-center gap-2 rounded-md px-2 py-1 text-left text-sm hover:bg-muted',
                          active && 'bg-muted font-medium',
                        )}
                        onClick={() => {
                          setSelected({ schema: s.name, table: tb.name })
                        }}
                      >
                        {tb.kind === 'table' ? (
                          <TableIcon
                            className="size-4 shrink-0"
                            aria-hidden="true"
                          />
                        ) : (
                          <EyeIcon
                            className="size-4 shrink-0"
                            aria-hidden="true"
                          />
                        )}
                        <span className="truncate">{tb.name}</span>
                      </button>
                    </li>
                  )
                })}
              </ul>
            </div>
          ))}
        </nav>
      </aside>

      <section className="min-w-0">
        {current ? (
          <TableDetail
            databaseName={databaseName}
            schema={current.schema}
            table={current.table}
          />
        ) : (
          <p className="rounded-lg border border-dashed border-border p-8 text-center text-sm text-muted-foreground">
            {t('viewer.explorer.selectTable')}
          </p>
        )}
      </section>
    </div>
  )
}

function TableDetail({
  databaseName,
  schema,
  table,
}: {
  databaseName: string
  schema: string
  table: DbTable
}) {
  const { t } = useTranslation('databases')
  return (
    <div className="space-y-3">
      <div>
        <h2 className="text-base font-semibold">
          {schema}.{table.name}
        </h2>
        <p className="text-xs text-muted-foreground">
          {table.kind} |{' '}
          {t('viewer.explorer.estimate', { count: table.row_estimate })} |{' '}
          {formatBytes(table.size_bytes)}
        </p>
      </div>
      <Tabs defaultValue="data">
        <TabsList>
          <TabsTrigger value="data">{t('viewer.explorer.tabData')}</TabsTrigger>
          <TabsTrigger value="structure">
            {t('viewer.explorer.tabStructure')}
          </TabsTrigger>
        </TabsList>
        <TabsContent value="data">
          <TableDataView
            key={`${schema}.${table.name}`}
            databaseName={databaseName}
            schema={schema}
            table={table}
          />
        </TabsContent>
        <TabsContent value="structure" className="space-y-4">
          <StructureTables table={table} />
        </TabsContent>
      </Tabs>
    </div>
  )
}

function StructureTables({ table }: { table: DbTable }) {
  const { t } = useTranslation('databases')
  return (
    <>
      <div className="overflow-auto rounded-lg border border-border">
        <table className="w-full text-sm">
          <caption className="sr-only">{t('viewer.explorer.columns')}</caption>
          <thead className="bg-muted/60 text-left text-xs text-muted-foreground">
            <tr>
              <th className="px-3 py-2 font-medium">
                {t('viewer.explorer.colName')}
              </th>
              <th className="px-3 py-2 font-medium">
                {t('viewer.explorer.colType')}
              </th>
              <th className="px-3 py-2 font-medium">
                {t('viewer.explorer.colNullable')}
              </th>
              <th className="px-3 py-2 font-medium">
                {t('viewer.explorer.colDefault')}
              </th>
            </tr>
          </thead>
          <tbody>
            {table.columns.map((c) => (
              <tr key={c.name} className="border-t border-border/60">
                <td className="px-3 py-1.5 font-mono text-xs">
                  <span className="inline-flex items-center gap-1">
                    {c.primary_key ? (
                      <KeyIcon
                        className="size-3.5 text-primary"
                        aria-label={t('viewer.explorer.primaryKey')}
                      />
                    ) : null}
                    {c.name}
                  </span>
                </td>
                <td className="px-3 py-1.5 font-mono text-xs">{c.type}</td>
                <td className="px-3 py-1.5 text-xs">
                  {c.nullable ? 'YES' : 'NO'}
                </td>
                <td className="max-w-64 truncate px-3 py-1.5 font-mono text-xs text-muted-foreground">
                  {c.default}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <div>
        <h3 className="mb-1.5 text-sm font-medium">
          {t('viewer.explorer.indexes')}
        </h3>
        {table.indexes.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            {t('viewer.explorer.noIndexes')}
          </p>
        ) : (
          <ul className="space-y-1.5">
            {table.indexes.map((ix) => (
              <li
                key={ix.name}
                className="rounded-md border border-border/60 px-3 py-1.5"
              >
                <p className="text-sm font-medium">
                  {ix.name}
                  {ix.primary ? ` (${t('viewer.explorer.primaryKey')})` : ''}
                  {ix.unique && !ix.primary
                    ? ` (${t('viewer.explorer.unique')})`
                    : ''}
                </p>
                <p className="break-all font-mono text-xs text-muted-foreground">
                  {ix.definition}
                </p>
              </li>
            ))}
          </ul>
        )}
      </div>
    </>
  )
}
