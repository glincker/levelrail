import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  ChevronRightGlassIcon,
  DatabaseGlassIcon,
  UploadGlassIcon,
} from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import type { DatabaseListEntry } from '../types/databaseDetail'
import { logoIdForImage } from '../lib/imageLogo'
import { StatusLabel } from './AppRow'
import { ManagedByStackTag } from './stacks/ManagedByStackNotice'
import { PlacementLabel } from './PlacementLabel'
import { TemplateLogo } from './TemplateLogo'

// Same anatomy as APP_LIST_GRID: avatar, name with status beneath, then
// lg-only columns, trailing chevron. Shared by the sticky header and rows.
export const DATABASE_LIST_GRID =
  'grid grid-cols-[2.75rem_minmax(0,1fr)_1rem] lg:grid-cols-[2.75rem_minmax(0,1.6fr)_8rem_9rem_7rem_1rem] items-center gap-3'

function EngineLogo({ engine }: { engine: string }) {
  return (
    <span className="flex size-10 shrink-0 items-center justify-center overflow-hidden rounded-lg bg-muted text-muted-foreground">
      <TemplateLogo
        id={logoIdForImage(engine) ?? ''}
        className="size-4"
        fallback={<DatabaseGlassIcon className="size-5" aria-hidden="true" />}
      />
    </span>
  )
}

function engineLabel(database: DatabaseListEntry): string {
  return database.external
    ? `${database.engine} ${database.external.host}:${database.external.port}`
    : `${database.engine} ${database.version}`
}

// The row links by database.name, the only identifier databaseResource
// carries; the status comes from the batched GET /api/v1/databases field,
// so there is no per-row fetch.
export function DatabaseRow({ database }: { database: DatabaseListEntry }) {
  const { t } = useTranslation('databases')
  const backupsOn = Boolean(database.backup_schedule)
  return (
    <div
      className={`${DATABASE_LIST_GRID} group/row relative h-full w-full border-b border-border px-4 py-3 transition-colors hover:bg-muted/60`}
    >
      <EngineLogo engine={database.engine} />

      <span className="flex min-w-0 flex-col justify-center gap-1">
        <Link
          to="/databases/$name"
          params={{ name: database.name }}
          className="truncate text-sm font-medium text-foreground after:absolute after:inset-0"
        >
          {database.name}
        </Link>
        <span className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
          {database.provision_state ? (
            <span
              className={
                database.provision_state === 'failed'
                  ? 'text-xs text-destructive'
                  : 'text-xs text-muted-foreground'
              }
              title={database.provision_reason}
              data-testid="provision-state"
            >
              {database.provision_state === 'failed'
                ? t('list.failedToCreate')
                : t('list.notCreated')}
            </span>
          ) : (
            <StatusLabel status={database.status} />
          )}
          {database.external ? (
            <Badge
              variant="outline"
              className="px-1.5 py-0"
              title={t('external.badgeHint')}
            >
              {t('external.badge')}
            </Badge>
          ) : null}
          <ManagedByStackTag stack={database.managed_by_stack} />
          <span className="min-w-0 truncate font-mono text-xs text-muted-foreground lg:hidden">
            {engineLabel(database)}
          </span>
        </span>
      </span>

      <span className="hidden min-w-0 flex-col lg:flex">
        <span className="text-xs font-medium text-foreground capitalize">
          {database.engine}
        </span>
        <span className="truncate font-mono text-xs text-muted-foreground">
          {database.external
            ? `${database.external.host}:${database.external.port}`
            : database.version}
        </span>
      </span>

      <span className="hidden min-w-0 items-center gap-1.5 text-xs lg:flex">
        <UploadGlassIcon
          className="size-5 shrink-0 text-muted-foreground"
          aria-hidden="true"
        />
        <span
          className={
            backupsOn ? 'truncate text-foreground' : 'text-muted-foreground/60'
          }
          title={database.backup_schedule}
        >
          {backupsOn ? t('list.backupsOn') : t('list.backupsOff')}
        </span>
      </span>

      <span className="hidden min-w-0 truncate text-xs text-muted-foreground lg:block">
        {database.external ? (
          <span className="text-muted-foreground">-</span>
        ) : (
          <PlacementLabel
            nodeId={database.node_id}
            nodeName={database.node_name}
            nodeIsLocal={database.node_is_local}
          />
        )}
      </span>

      <ChevronRightGlassIcon
        className="size-5 shrink-0 justify-self-end text-muted-foreground/50"
        aria-hidden="true"
      />
    </div>
  )
}

// Matches DATABASE_LIST_GRID so the skeleton does not shift when rows land.
export function RowSkeleton() {
  return (
    <div
      className={`${DATABASE_LIST_GRID} border-b border-border px-4 py-3`}
      aria-hidden="true"
    >
      <div className="size-10 animate-pulse rounded-lg bg-muted" />
      <div className="h-4 w-32 animate-pulse rounded bg-muted" />
      <div className="hidden h-4 w-16 animate-pulse rounded bg-muted lg:block" />
      <div className="hidden h-4 w-20 animate-pulse rounded bg-muted lg:block" />
      <div className="hidden h-4 w-14 animate-pulse rounded bg-muted lg:block" />
      <div className="h-4 w-4 animate-pulse justify-self-end rounded bg-muted" />
    </div>
  )
}
