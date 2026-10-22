import { useMemo, useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  ROW_LINK_OVERLAY,
  ResponsiveTable,
  type ResponsiveColumn,
} from '@/components/ui/responsive-table'
import type { DatabaseListEntry } from '../types/databaseDetail'
import { StatusLabel } from './AppRow'
import { BrandTile } from './BrandLogoBadge'
import { PlacementLabel } from './PlacementLabel'

const MIB = 1024 * 1024
const GIB = 1024 * MIB
const DEFAULT_PAGE_SIZE = 50

function pageSize(): number {
  const raw = Number(
    import.meta.env.VITE_DATABASES_PAGE_SIZE ?? DEFAULT_PAGE_SIZE,
  )
  return Number.isInteger(raw) && raw > 0 ? raw : DEFAULT_PAGE_SIZE
}

function Dot({ on }: { on: boolean }) {
  return (
    <span
      aria-hidden="true"
      className={`size-2 shrink-0 rounded-full ${on ? 'bg-emerald-500' : 'bg-muted-foreground/40'}`}
    />
  )
}

export function DatabasesTable({
  databases,
}: {
  databases: readonly DatabaseListEntry[]
}) {
  const { t } = useTranslation('databases')
  const [shown, setShown] = useState(pageSize)
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
        id: 'size',
        header: t('list.colSize'),
        priority: 3,
        width: 'w-28',
        cell: (db) => {
          const bytes = db.resources?.memory_bytes ?? 0
          if (bytes <= 0) {
            return (
              <span className="text-xs text-muted-foreground">
                {t('list.sizeShared')}
              </span>
            )
          }
          const label =
            bytes >= GIB
              ? t('list.sizeGb', { value: Number((bytes / GIB).toFixed(1)) })
              : t('list.sizeMb', { value: Math.round(bytes / MIB) })
          return <span className="font-mono text-xs">{label}</span>
        },
      },
      {
        id: 'backups',
        header: t('list.colBackups'),
        priority: 3,
        width: 'w-40',
        cell: (db) => {
          const on = Boolean(db.backup_schedule)
          return (
            <span className="flex items-center gap-2 text-xs">
              <Dot on={on} />
              <span
                className={on ? 'text-foreground' : 'text-muted-foreground'}
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
  const visible = databases.slice(0, shown)
  return (
    <div className="space-y-3">
      <ResponsiveTable
        rows={visible}
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
      {visible.length < databases.length ? (
        <div className="flex items-center justify-between gap-3 text-sm text-muted-foreground">
          <span>
            {t('list.showing', {
              shown: visible.length,
              total: databases.length,
            })}
          </span>
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              setShown((n) => n + pageSize())
            }}
          >
            {t('list.showMore')}
          </Button>
        </div>
      ) : null}
    </div>
  )
}
