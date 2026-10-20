import { useTranslation } from 'react-i18next'
import {
  ArrowRightIcon,
  LinkSimpleIcon,
  TableIcon,
} from '@phosphor-icons/react/dist/ssr'
import type { DbForeignKey } from '../../../types/databaseViewer'

function TableNode({
  schema,
  table,
  detail,
  onOpen,
}: {
  schema: string
  table: string
  detail: string
  onOpen: (schema: string, table: string) => void
}) {
  return (
    <button
      type="button"
      onClick={() => {
        onOpen(schema, table)
      }}
      className="flex w-full min-w-0 flex-col rounded-lg border border-border bg-card px-3 py-2 text-left text-sm hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring/60 focus-visible:outline-none"
    >
      <span className="flex items-center gap-1.5 font-medium">
        <TableIcon className="size-3.5 shrink-0" aria-hidden="true" />
        <span className="truncate">
          {schema}.{table}
        </span>
      </span>
      <span className="truncate font-mono text-xs text-muted-foreground">
        {detail}
      </span>
    </button>
  )
}

// A one-hop picture of how a table connects: tables that point at it on
// the left, tables it points at on the right. Every box opens that table.
export function RelationsGraph({
  schema,
  table,
  outgoing,
  incoming,
  onOpen,
}: {
  schema: string
  table: string
  outgoing: DbForeignKey[]
  incoming: DbForeignKey[]
  onOpen: (schema: string, table: string) => void
}) {
  const { t } = useTranslation('databases')
  if (outgoing.length === 0 && incoming.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">
        {t('viewer.structure.noRelations')}
      </p>
    )
  }
  return (
    <div
      role="group"
      aria-label={t('viewer.structure.relations')}
      className="grid items-center gap-3 md:grid-cols-[1fr_auto_1fr_auto_1fr]"
    >
      <ul className="space-y-2">
        {incoming.length === 0 ? (
          <li className="text-xs text-muted-foreground">
            {t('viewer.structure.noIncoming')}
          </li>
        ) : (
          incoming.map((fk) => (
            <li key={`${fk.schema}.${fk.table}.${fk.name}`}>
              <TableNode
                schema={fk.schema}
                table={fk.table}
                detail={`${fk.columns.join(', ')}`}
                onOpen={onOpen}
              />
            </li>
          ))
        )}
      </ul>
      <ArrowRightIcon
        className="hidden size-4 text-muted-foreground md:block"
        aria-hidden="true"
      />
      <div className="flex items-center gap-1.5 rounded-lg border-2 border-primary/40 bg-primary/5 px-3 py-3 text-sm font-semibold">
        <LinkSimpleIcon className="size-4 shrink-0" aria-hidden="true" />
        <span className="truncate">
          {schema}.{table}
        </span>
      </div>
      <ArrowRightIcon
        className="hidden size-4 text-muted-foreground md:block"
        aria-hidden="true"
      />
      <ul className="space-y-2">
        {outgoing.length === 0 ? (
          <li className="text-xs text-muted-foreground">
            {t('viewer.structure.noOutgoing')}
          </li>
        ) : (
          outgoing.map((fk) => (
            <li key={fk.name}>
              <TableNode
                schema={fk.ref_schema}
                table={fk.ref_table}
                detail={`${fk.columns.join(', ')} -> ${fk.ref_columns.join(', ')}`}
                onOpen={onOpen}
              />
            </li>
          ))
        )}
      </ul>
    </div>
  )
}
