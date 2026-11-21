import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { Input } from '@/components/ui/input'
import { messageOf } from '../../lib/cloudAwsServices'
import { cacheCanChange } from '../../lib/cloudElastiCache'
import {
  useAwsCacheAction,
  useAwsCacheClusters,
  useAwsCacheGroups,
} from '../../queries/cloudElastiCache'
import { StateLine } from './AwsServiceShared'

export function ElastiCacheCard({
  id,
  canDelete,
}: Readonly<{ id: string; canDelete: boolean }>) {
  const { t } = useTranslation('cloudAccounts')
  const groups = useAwsCacheGroups(id)
  const clusters = useAwsCacheClusters(id)
  const act = useAwsCacheAction(id)
  const [deleting, setDeleting] = useState<string | null>(null)
  const [finalSnapshot, setFinalSnapshot] = useState('')
  const empty =
    groups.data?.length === 0 && clusters.data?.length === 0
      ? t('services.elasticache.empty')
      : undefined
  return (
    <div className="space-y-2">
      {act.isError ? (
        <p role="alert" className="text-sm text-destructive">
          {messageOf(act.error)}
        </p>
      ) : null}
      <StateLine
        pending={groups.isPending}
        error={groups.error}
        empty={empty}
      />
      <h4 className="text-xs font-medium text-muted-foreground">
        {t('services.elasticache.groups')}
      </h4>
      <ul className="space-y-1">
        {(groups.data ?? []).map((g) => (
          <li key={g.id} className="flex flex-wrap items-center gap-2 text-sm">
            <span className="font-mono text-xs">{g.id}</span>
            <span className="text-xs text-muted-foreground">
              {g.status} · {g.node_type}
            </span>
            {cacheCanChange(g) ? (
              <span className="ml-auto flex gap-1">
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => {
                    act.mutate({ kind: 'snapshot', id: g.id })
                  }}
                >
                  {t('services.elasticache.snapshot')}
                </Button>
                {canDelete ? (
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => {
                      setFinalSnapshot('')
                      setDeleting(g.id)
                    }}
                  >
                    {t('services.elasticache.delete')}
                  </Button>
                ) : null}
              </span>
            ) : null}
          </li>
        ))}
      </ul>
      <h4 className="text-xs font-medium text-muted-foreground">
        {t('services.elasticache.clusters')}
      </h4>
      <ul className="space-y-1">
        {(clusters.data ?? []).map((k) => (
          <li key={k.id} className="flex flex-wrap items-center gap-2 text-sm">
            <span className="font-mono text-xs">{k.id}</span>
            <span className="text-xs text-muted-foreground">
              {k.status} · {k.engine} · {k.node_type}
            </span>
            {cacheCanChange(k) ? (
              <Button
                size="sm"
                variant="outline"
                className="ml-auto"
                onClick={() => {
                  act.mutate({ kind: 'reboot', id: k.id })
                }}
              >
                {t('services.elasticache.reboot')}
              </Button>
            ) : null}
          </li>
        ))}
      </ul>
      {deleting === null ? null : (
        <ConfirmDialog
          open
          onOpenChange={(open) => {
            if (!open) setDeleting(null)
          }}
          title={t('services.elasticache.deleteTitle')}
          description={t('services.elasticache.deleteBody')}
          confirmLabel={t('services.elasticache.delete')}
          requireTyped={deleting}
          pending={act.isPending}
          error={act.isError ? messageOf(act.error) : null}
          onConfirm={() => {
            act.mutate(
              {
                kind: 'delete',
                id: deleting,
                finalSnapshot: finalSnapshot.trim(),
              },
              {
                onSuccess: () => {
                  setDeleting(null)
                },
              },
            )
          }}
        >
          <Input
            value={finalSnapshot}
            aria-label={t('services.elasticache.finalSnapshot')}
            placeholder={t('services.elasticache.finalSnapshot')}
            onChange={(e) => {
              setFinalSnapshot(e.target.value)
            }}
          />
        </ConfirmDialog>
      )}
    </div>
  )
}
