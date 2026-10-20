import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { StatusPill } from '@/components/kit'
import { Button } from '../ui/button'
import { ConfirmDialog } from '../ui/confirm-dialog'
import { toast } from '../ui/toast'
import {
  restorePointsQueryOptions,
  useRestoreRestorePoint,
  type RestorePoint,
} from '../../queries/restorePoints'

const BYTES_PER_MIB = 1024 * 1024

function RestorePointRow({
  point,
  onRestore,
}: {
  point: RestorePoint
  onRestore: (p: RestorePoint) => void
}) {
  const { t } = useTranslation('updates')
  return (
    <li className="flex flex-wrap items-center gap-2 rounded-md border border-border px-3 py-2 text-sm">
      <span className="font-mono text-xs">{point.version}</span>
      <span className="text-xs text-muted-foreground">
        {new Date(point.created_at).toLocaleString()}
      </span>
      <span className="text-xs text-muted-foreground">
        {t('restorePoints.size', {
          mb: Math.max(1, Math.round(point.size_bytes / BYTES_PER_MIB)),
        })}
      </span>
      <StatusPill
        tone={point.database_restore_needed ? 'warning' : 'neutral'}
        label={t(
          point.database_restore_needed
            ? 'restorePoints.withData'
            : 'restorePoints.binaryOnly',
        )}
        size="sm"
      />
      <span className="flex-1" />
      {point.can_restore ? (
        <Button size="sm" variant="outline" onClick={() => onRestore(point)}>
          {t('restorePoints.restore')}
        </Button>
      ) : (
        <span className="text-xs text-muted-foreground">
          {point.cannot_restore_reason}
        </span>
      )}
    </li>
  )
}

export function RestorePoints() {
  const { t } = useTranslation('updates')
  const points = useQuery(restorePointsQueryOptions())
  const restore = useRestoreRestorePoint()
  const [target, setTarget] = useState<RestorePoint | null>(null)
  const list = points.data ?? []

  return (
    <div className="space-y-2">
      <h3 className="text-sm font-medium">{t('restorePoints.title')}</h3>
      <p className="text-xs text-muted-foreground">
        {t('restorePoints.description')}
      </p>
      {points.isError ? (
        <p className="text-sm text-destructive">{points.error.message}</p>
      ) : list.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          {t('restorePoints.empty')}
        </p>
      ) : (
        <ul className="space-y-2" aria-label={t('restorePoints.title')}>
          {list.map((p) => (
            <RestorePointRow key={p.id} point={p} onRestore={setTarget} />
          ))}
        </ul>
      )}
      {target ? (
        <ConfirmDialog
          open
          onOpenChange={(open) => {
            if (!open) setTarget(null)
          }}
          title={t('restorePoints.confirmTitle', { version: target.version })}
          description={t(
            target.database_restore_needed
              ? 'restorePoints.confirmWithData'
              : 'restorePoints.confirmBinaryOnly',
            {
              time: new Date(target.created_at).toLocaleString(),
            },
          )}
          confirmLabel={t('restorePoints.restore')}
          pendingLabel={t('restorePoints.pending')}
          tone="destructive"
          requireTyped={target.id}
          pending={restore.isPending}
          error={restore.isError ? restore.error.message : null}
          onConfirm={() => {
            restore.mutate(target.id, {
              onSuccess: () => {
                toast.add({
                  title: t('restorePoints.started', {
                    version: target.version,
                  }),
                  type: 'success',
                })
                setTarget(null)
              },
            })
          }}
        />
      ) : null}
    </div>
  )
}
