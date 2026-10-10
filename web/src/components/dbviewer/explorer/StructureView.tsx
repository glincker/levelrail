import { useTranslation } from 'react-i18next'
import {
  CopyIcon,
  KeyIcon,
  LinkSimpleIcon,
  TerminalWindowIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { columnKind } from '../../../lib/explorerData'
import type { DbForeignKey, DbStructure } from '../../../types/databaseViewer'
import { ColumnTypeIcon } from './ColumnTypeIcon'
import { RelationsGraph } from './RelationsGraph'
import { useCopy } from './useCopy'

function fkFor(column: string, fks: DbForeignKey[]): DbForeignKey | undefined {
  return fks.find((fk) => fk.columns.includes(column))
}

export function StructureView({
  structure,
  loading,
  error,
  onOpenTable,
  onOpenConsole,
}: {
  structure: DbStructure | undefined
  loading: boolean
  error: Error | null
  onOpenTable: (schema: string, table: string) => void
  onOpenConsole: () => void
}) {
  const { t } = useTranslation('databases')
  const copy = useCopy()

  if (loading) return <Skeleton className="h-48 w-full" />
  if (error) {
    return (
      <p className="text-sm text-destructive" role="alert">
        {error.message}
      </p>
    )
  }
  if (!structure) return null

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center gap-2">
        <Button type="button" size="sm" onClick={onOpenConsole}>
          <TerminalWindowIcon className="size-3.5" aria-hidden="true" />
          {t('viewer.structure.openConsole')}
        </Button>
      </div>

      <section className="space-y-2">
        <h3 className="text-sm font-medium">{t('viewer.explorer.columns')}</h3>
        <div className="overflow-auto rounded-lg border border-border">
          <table className="w-full text-sm">
            <caption className="sr-only">
              {t('viewer.explorer.columns')}
            </caption>
            <thead className="bg-muted/60 text-left text-xs text-muted-foreground">
              <tr>
                <th scope="col" className="px-3 py-2 font-medium">
                  {t('viewer.explorer.colName')}
                </th>
                <th scope="col" className="px-3 py-2 font-medium">
                  {t('viewer.explorer.colType')}
                </th>
                <th scope="col" className="px-3 py-2 font-medium">
                  {t('viewer.explorer.colNullable')}
                </th>
                <th scope="col" className="px-3 py-2 font-medium">
                  {t('viewer.explorer.colDefault')}
                </th>
                <th scope="col" className="px-3 py-2 font-medium">
                  {t('viewer.structure.references')}
                </th>
              </tr>
            </thead>
            <tbody>
              {structure.columns.map((c) => {
                const fk = fkFor(c.name, structure.foreign_keys)
                const refCol = fk
                  ? fk.ref_columns[fk.columns.indexOf(c.name)]
                  : undefined
                return (
                  <tr key={c.name} className="border-t border-border/60">
                    <td className="px-3 py-1.5 font-mono text-xs">
                      <span className="inline-flex items-center gap-1.5">
                        <ColumnTypeIcon kind={columnKind(c.type)} />
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
                      {c.nullable
                        ? t('viewer.structure.yes')
                        : t('viewer.structure.no')}
                    </td>
                    <td className="max-w-64 truncate px-3 py-1.5 font-mono text-xs text-muted-foreground">
                      {c.default}
                    </td>
                    <td className="px-3 py-1.5 text-xs">
                      {fk ? (
                        <button
                          type="button"
                          className="inline-flex items-center gap-1 font-mono text-primary hover:underline"
                          onClick={() => {
                            onOpenTable(fk.ref_schema, fk.ref_table)
                          }}
                        >
                          <LinkSimpleIcon
                            className="size-3.5"
                            aria-hidden="true"
                          />
                          {fk.ref_table}.{refCol}
                        </button>
                      ) : null}
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      </section>

      <section className="space-y-2">
        <h3 className="text-sm font-medium">{t('viewer.explorer.indexes')}</h3>
        {structure.indexes.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            {t('viewer.explorer.noIndexes')}
          </p>
        ) : (
          <ul className="space-y-1.5">
            {structure.indexes.map((ix) => (
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
      </section>

      <section className="space-y-2">
        <h3 className="text-sm font-medium">
          {t('viewer.structure.relations')}
        </h3>
        <RelationsGraph
          schema={structure.schema}
          table={structure.name}
          outgoing={structure.foreign_keys}
          incoming={structure.referenced_by}
          onOpen={onOpenTable}
        />
      </section>

      {structure.ddl ? (
        <section className="space-y-2">
          <div className="flex items-center justify-between">
            <h3 className="text-sm font-medium">{t('viewer.structure.ddl')}</h3>
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => {
                copy(structure.ddl, 'viewer.structure.ddlCopied')
              }}
            >
              <CopyIcon className="size-3.5" aria-hidden="true" />
              {t('viewer.grid.copy')}
            </Button>
          </div>
          <pre className="max-h-96 overflow-auto rounded-lg border border-border bg-muted/40 p-3 font-mono text-xs">
            {structure.ddl}
          </pre>
        </section>
      ) : null}
    </div>
  )
}
