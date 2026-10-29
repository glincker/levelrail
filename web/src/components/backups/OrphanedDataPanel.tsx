import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { toast } from '@/components/ui/toast'
import { TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { formatBytes, formatDate } from '../../lib/format'
import {
  useReclaimOrphan,
  useStorageOrphans,
} from '../../queries/backupCoverage'
import type { StorageOrphan } from '../../types/backupCoverage'

function ownerLabel(o: StorageOrphan): string {
  const b = o.belonged_to
  const name = b.app_name ? `${b.app_name}/${b.name ?? ''}` : (b.name ?? '')
  return name || o.name
}

function OrphanRow({
  orphan,
  onReclaim,
}: {
  orphan: StorageOrphan
  onReclaim: (o: StorageOrphan) => void
}) {
  const { t } = useTranslation('backups')
  const [open, setOpen] = useState(false)
  const detailId = `orphan-${orphan.id}`
  return (
    <li className="space-y-2 border-b border-border px-3 py-3 last:border-b-0">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="min-w-0">
          <p className="truncate font-medium">{ownerLabel(orphan)}</p>
          <p className="text-xs text-muted-foreground">
            {t(`orphans.role.${orphan.belonged_to.role}`, {
              defaultValue: orphan.belonged_to.role,
            })}
            {' · '}
            {orphan.size_bytes === undefined
              ? t('orphans.sizeUnknown')
              : formatBytes(orphan.size_bytes)}
            {orphan.age_days !== undefined
              ? ` · ${t('orphans.age', { count: orphan.age_days })}`
              : ''}
          </p>
        </div>
        <div className="flex items-center gap-2">
          {orphan.backup_count === 0 ? (
            <Badge variant="warning">{t('orphans.noBackup')}</Badge>
          ) : (
            <Badge variant="success">
              {t('orphans.hasBackup', { count: orphan.backup_count })}
            </Badge>
          )}
          <Button
            type="button"
            size="sm"
            variant="outline"
            aria-expanded={open}
            aria-controls={detailId}
            onClick={() => {
              setOpen((v) => !v)
            }}
          >
            {t('orphans.inspect')}
          </Button>
          <Button
            type="button"
            size="sm"
            variant="destructive"
            onClick={() => {
              onReclaim(orphan)
            }}
          >
            {t('orphans.reclaim')}
          </Button>
        </div>
      </div>
      {open ? (
        <dl
          id={detailId}
          className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 rounded-md bg-muted/50 p-3 text-xs"
        >
          <dt className="text-muted-foreground">{t('orphans.volume')}</dt>
          <dd className="break-all font-mono">{orphan.name}</dd>
          <dt className="text-muted-foreground">{t('orphans.belongedTo')}</dt>
          <dd>
            {t(`orphans.kind.${orphan.belonged_to.kind}`, {
              defaultValue: orphan.belonged_to.kind,
            })}{' '}
            {ownerLabel(orphan)}
          </dd>
          <dt className="text-muted-foreground">{t('orphans.created')}</dt>
          <dd>{formatDate(orphan.created_at, '-')}</dd>
          <dt className="text-muted-foreground">{t('orphans.lastBackup')}</dt>
          <dd>
            {orphan.last_backup
              ? `${formatDate(orphan.last_backup.at, '-')} (${formatBytes(orphan.last_backup.size_bytes)})`
              : t('orphans.noBackupDetail')}
          </dd>
        </dl>
      ) : null}
    </li>
  )
}

/** OrphanedDataPanel lists data kept after its database or app was deleted, and reclaims it after a typed confirmation. */
export function OrphanedDataPanel() {
  const { t } = useTranslation('backups')
  const orphans = useStorageOrphans()
  const reclaim = useReclaimOrphan()
  const [target, setTarget] = useState<StorageOrphan | null>(null)
  const list = orphans.data?.orphans ?? []

  if (orphans.isError && orphans.error.status === 501) return null

  return (
    <section
      id="orphaned-data"
      className="space-y-3 rounded-lg border border-border p-4"
      aria-labelledby="orphaned-data-title"
    >
      <div>
        <h2 id="orphaned-data-title" className="text-base font-medium">
          {t('orphans.title')}
        </h2>
        <p className="text-sm text-muted-foreground">
          {t('orphans.description')}
        </p>
      </div>
      {orphans.isError ? (
        <p className="text-sm text-destructive">
          {t('orphans.loadFailed', { error: orphans.error.message })}
        </p>
      ) : list.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t('orphans.empty')}</p>
      ) : (
        <>
          {orphans.data &&
          orphans.data.total_bytes >= orphans.data.threshold_bytes ? (
            <Alert>
              <TriangleAlertGlassIcon
                className="text-tone-warning"
                size="1.4em"
                aria-hidden="true"
              />
              <AlertDescription>
                {t('orphans.large', {
                  size: formatBytes(orphans.data.total_bytes),
                })}
              </AlertDescription>
            </Alert>
          ) : null}
          <ul className="overflow-hidden rounded-lg border border-border">
            {list.map((o) => (
              <OrphanRow key={o.id} orphan={o} onReclaim={setTarget} />
            ))}
          </ul>
        </>
      )}
      {target ? (
        <ConfirmDialog
          open
          onOpenChange={(next) => {
            if (!next) {
              setTarget(null)
              reclaim.reset()
            }
          }}
          tone="destructive"
          title={t('orphans.confirmTitle', { name: ownerLabel(target) })}
          description={t('orphans.confirmDescription')}
          consequences={[
            target.backup_count === 0
              ? t('orphans.confirmNoBackup')
              : t('orphans.confirmHasBackup', { count: target.backup_count }),
          ]}
          requireTyped={target.id}
          confirmLabel={t('orphans.confirmAction')}
          pendingLabel={t('orphans.reclaiming')}
          pending={reclaim.isPending}
          error={reclaim.isError ? reclaim.error.message : null}
          onConfirm={() => {
            reclaim.mutate(target.id, {
              onSuccess: (res) => {
                toast.add({
                  title: t('orphans.reclaimed', {
                    size: formatBytes(res.reclaimed_bytes),
                  }),
                  type: 'success',
                })
                setTarget(null)
              },
            })
          }}
        />
      ) : null}
    </section>
  )
}
