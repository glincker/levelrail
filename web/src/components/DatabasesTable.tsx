import { useMemo } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { CloudArrowUpIcon } from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import {
  ROW_LINK_OVERLAY,
  ResponsiveTable,
  type ResponsiveColumn,
} from '@/components/ui/responsive-table'
import type { DatabaseListEntry } from '../types/databaseDetail'
import { StatusLabel } from './AppRow'
import { BrandTile } from './BrandLogoBadge'
import { PlacementLabel } from './PlacementLabel'

export function DatabasesTable({
  databases,
}: {
  databases: readonly DatabaseListEntry[]
}) {
  const { t } = useTranslation('databases')
  const columns = useMemo<ResponsiveColumn<DatabaseListEntry>[]>(
    () => [
      {
        id: 'name',
        primary: true,
        header: t('list.colName'),
        cell: (db) => (
          <span className="flex min-w-0 flex-col gap-1">
            <Link
              to="/databases/$name"
              params={{ name: db.name }}
              className={`truncate text-sm font-medium text-foreground ${ROW_LINK_OVERLAY}`}
            >
              {db.name}
            </Link>
            <span className="flex flex-wrap items-center gap-x-2 gap-y-1">
              <StatusLabel status={db.status} />
              {db.external ? (
                <Badge
                  variant="outline"
                  className="px-1.5 py-0"
                  title={t('external.badgeHint')}
                >
                  {t('external.badge')}
                </Badge>
              ) : null}
            </span>
          </span>
        ),
      },
      {
        id: 'engine',
        header: t('list.colEngine'),
        priority: 2,
        width: 'w-40',
        cell: (db) => (
          <span className="flex min-w-0 flex-col">
            <span className="text-xs font-medium text-foreground capitalize">
              {db.engine}
            </span>
            <span className="truncate font-mono text-xs text-muted-foreground">
              {db.external
                ? `${db.external.host}:${db.external.port}`
                : db.version}
            </span>
          </span>
        ),
      },
      {
        id: 'backups',
        header: t('list.colBackups'),
        priority: 3,
        width: 'w-36',
        cell: (db) => {
          const on = Boolean(db.backup_schedule)
          return (
            <span className="flex items-center gap-1.5 text-xs">
              <CloudArrowUpIcon
                className="size-3.5 shrink-0 text-muted-foreground"
                aria-hidden="true"
              />
              <span
                className={on ? 'text-foreground' : 'text-muted-foreground/70'}
                title={db.backup_schedule}
              >
                {on ? t('list.backupsOn') : t('list.backupsOff')}
              </span>
            </span>
          )
        },
      },
      {
        id: 'node',
        header: t('list.colNode'),
        priority: 2,
        width: 'w-36',
        cell: (db) =>
          db.external ? (
            <span className="text-xs text-muted-foreground">-</span>
          ) : (
            <span className="block truncate text-xs text-muted-foreground">
              <PlacementLabel
                nodeId={db.node_id}
                nodeName={db.node_name}
                nodeIsLocal={db.node_is_local}
              />
            </span>
          ),
      },
    ],
    [t],
  )
  return (
    <ResponsiveTable
      rows={databases}
      columns={columns}
      getRowId={(db) => db.name}
      ariaLabel={t('list.tableLabel')}
      leading={{
        header: <span className="sr-only">{t('list.colEngine')}</span>,
        cell: (db) => (
          <BrandTile name={db.engine} label={db.engine} size="sm" />
        ),
      }}
    />
  )
}
