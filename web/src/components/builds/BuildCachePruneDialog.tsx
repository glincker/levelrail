import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { toast } from '@/components/ui/toast'
import { usePruneBuildCache } from '../../queries/buildCenter'
import { sizeLabel } from './buildFormat'

interface Props {
  node: string
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function BuildCachePruneDialog({ node, open, onOpenChange }: Props) {
  const { t } = useTranslation('buildCenter')
  const prune = usePruneBuildCache(node)
  const { mutate, reset } = prune

  useEffect(() => {
    if (open) mutate(false)
    else reset()
  }, [open, mutate, reset])

  const preview = prune.data && !prune.data.applied ? prune.data : null

  return (
    <ConfirmDialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('prune.title')}
      description={
        preview
          ? t('prune.description', {
              count: preview.candidate_count,
              hours: preview.older_than_hours,
              size: sizeLabel(preview.would_free_bytes),
            })
          : t('prune.loading')
      }
      consequences={[t('prune.consequences')]}
      confirmLabel={t('prune.confirm', {
        size: sizeLabel(preview?.would_free_bytes),
      })}
      pendingLabel={t('prune.pending')}
      tone="destructive"
      pending={prune.isPending}
      error={prune.error ? prune.error.message : null}
      onConfirm={() => {
        if (!preview || preview.candidate_count === 0) {
          onOpenChange(false)
          return
        }
        mutate(true, {
          onSuccess: (res) => {
            toast.add({
              title: t('prune.done', {
                count: res.removed_count,
                size: sizeLabel(res.freed_bytes),
              }),
              type: 'success',
            })
            onOpenChange(false)
          },
        })
      }}
    >
      {preview && preview.candidates.length > 0 ? (
        <ul className="max-h-48 overflow-y-auto rounded-lg border border-border text-sm">
          {preview.candidates.map((c) => (
            <li
              key={c.id}
              className="flex items-center justify-between gap-3 border-b border-border px-3 py-1.5 last:border-b-0"
            >
              <span className="truncate text-xs">
                {c.description || c.type}
              </span>
              <span className="shrink-0 text-xs text-muted-foreground tabular-nums">
                {sizeLabel(c.size_bytes)}
              </span>
            </li>
          ))}
        </ul>
      ) : null}
    </ConfirmDialog>
  )
}
