import { useMemo } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { HardDrivesIcon } from '@phosphor-icons/react/dist/ssr'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { SkeletonTile } from '@/components/kit'
import { useUsageSummary } from '../../queries/usageSummary'
import type { VolumeUsageItem } from '../../queries/usageSummary'
import { formatSize } from '../../lib/format'
import { Readout } from './Readout'

const ROW_CAP = 4

interface Owned {
  kind: 'app' | 'database'
  name: string
  bytes: number
}

function ownedTotals(items: VolumeUsageItem[]): Owned[] {
  const byOwner = new Map<string, Owned>()
  for (const v of items) {
    if (v.owner_kind === 'other' || !v.owner || v.size_bytes === undefined) {
      continue
    }
    const key = `${v.owner_kind}:${v.owner}`
    const cur = byOwner.get(key) ?? {
      kind: v.owner_kind,
      name: v.owner,
      bytes: 0,
    }
    cur.bytes += v.size_bytes
    byOwner.set(key, cur)
  }
  return [...byOwner.values()].sort((a, b) => b.bytes - a.bytes)
}

function OwnerLink({ kind, name }: { kind: 'app' | 'database'; name: string }) {
  const cls = 'min-w-0 flex-1 truncate font-medium hover:underline'
  return kind === 'app' ? (
    <Link to="/apps/$name" params={{ name }} className={cls}>
      {name}
    </Link>
  ) : (
    <Link to="/databases/$name" params={{ name }} className={cls}>
      {name}
    </Link>
  )
}

export function StorageUsageCard() {
  const { t } = useTranslation('dashboard')
  const q = useUsageSummary()
  const owned = useMemo(
    () => ownedTotals(q.data?.volumes.items ?? []),
    [q.data],
  )

  if (q.isPending) return <SkeletonTile className="h-40" />
  if (q.isError || !q.data) return null
  const { volumes, backups } = q.data
  if (volumes.not_configured && backups.count === 0) return null

  const dbBytes = owned
    .filter((o) => o.kind === 'database')
    .reduce((sum, o) => sum + o.bytes, 0)
  const appBytes = owned
    .filter((o) => o.kind === 'app')
    .reduce((sum, o) => sum + o.bytes, 0)

  return (
    <Card size="sm">
      <CardHeader className="space-y-0">
        <CardTitle className="flex items-center gap-1.5 text-sm font-medium">
          <HardDrivesIcon
            className="size-4 text-muted-foreground"
            aria-hidden="true"
          />
          {t('fleet.storage.title')}
        </CardTitle>
        <p className="text-xs text-muted-foreground">
          {t('fleet.storage.scope')}
        </p>
      </CardHeader>
      <CardContent className="space-y-4">
        <dl className="grid grid-cols-2 gap-3 sm:grid-cols-4">
          <Readout
            label={t('fleet.storage.databaseVolumes')}
            value={formatSize(dbBytes)}
          />
          <Readout
            label={t('fleet.storage.appVolumes')}
            value={formatSize(appBytes)}
            hint={
              volumes.unknown_count > 0
                ? t('fleet.storage.unknownSizes', {
                    count: volumes.unknown_count,
                  })
                : undefined
            }
          />
          <Readout
            label={t('fleet.storage.backups')}
            value={formatSize(backups.total_bytes)}
            hint={t('fleet.storage.backupCount', { count: backups.count })}
          />
          <Readout
            label={t('fleet.storage.total')}
            value={formatSize(volumes.total_bytes + backups.total_bytes)}
            hint={t('fleet.storage.totalHint')}
          />
        </dl>
        {owned.length > 0 || backups.items.length > 0 ? (
          <div className="grid gap-6 lg:grid-cols-2">
            <div className="space-y-1">
              <h3 className="text-sm font-medium">
                {t('fleet.storage.largestVolumes')}
              </h3>
              {owned.length === 0 ? (
                <p className="text-sm text-muted-foreground">
                  {t('fleet.storage.noVolumes')}
                </p>
              ) : (
                <ul className="divide-y divide-border/60">
                  {owned.slice(0, ROW_CAP).map((o) => (
                    <li
                      key={`${o.kind}:${o.name}`}
                      className="flex items-center gap-2 py-1.5 text-sm"
                    >
                      <OwnerLink kind={o.kind} name={o.name} />
                      <span className="shrink-0 text-xs text-muted-foreground">
                        {t(`fleet.reserved.kind.${o.kind}`)}
                      </span>
                      <span className="w-20 shrink-0 text-right text-xs tabular-nums">
                        {formatSize(o.bytes)}
                      </span>
                    </li>
                  ))}
                </ul>
              )}
            </div>
            <div className="space-y-1">
              <h3 className="text-sm font-medium">
                {t('fleet.storage.largestBackups')}
              </h3>
              {backups.items.length === 0 ? (
                <p className="text-sm text-muted-foreground">
                  {t('fleet.storage.noBackups')}
                </p>
              ) : (
                <ul className="divide-y divide-border/60">
                  {backups.items.slice(0, ROW_CAP).map((b) => (
                    <li
                      key={`${b.kind}:${b.name}`}
                      className="flex items-center gap-2 py-1.5 text-sm"
                    >
                      <span className="min-w-0 flex-1 truncate font-medium">
                        {b.name}
                      </span>
                      <span className="shrink-0 text-xs text-muted-foreground">
                        {t('fleet.storage.backupCount', { count: b.count })}
                      </span>
                      <span className="w-20 shrink-0 text-right text-xs tabular-nums">
                        {formatSize(b.bytes)}
                      </span>
                    </li>
                  ))}
                </ul>
              )}
            </div>
          </div>
        ) : null}
      </CardContent>
    </Card>
  )
}
