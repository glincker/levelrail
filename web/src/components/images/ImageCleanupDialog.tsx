import { useTranslation } from 'react-i18next'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { toast } from '@/components/ui/toast'
import { useImageGC, type ImageEntry } from '../../queries/imageInventory'
import { imageTitle, shortId, sizeLabel } from './imageFormat'

interface Props {
  node: string
  open: boolean
  onOpenChange: (open: boolean) => void
  candidates: ImageEntry[]
  keepPerApp: number
  reclaimBytes: number
}

export function ImageCleanupDialog({
  node,
  open,
  onOpenChange,
  candidates,
  keepPerApp,
  reclaimBytes,
}: Props) {
  const { t } = useTranslation('images')
  const gc = useImageGC(node)

  return (
    <ConfirmDialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('cleanup.title')}
      description={t('cleanup.description', { keep: keepPerApp })}
      consequences={[t('cleanup.consequences')]}
      confirmLabel={t('cleanup.button', { size: sizeLabel(reclaimBytes) })}
      pendingLabel={t('cleanup.pending')}
      tone="destructive"
      pending={gc.isPending}
      error={gc.error ? gc.error.message : null}
      onConfirm={() => {
        gc.mutate(false, {
          onSuccess: (res) => {
            toast.add({
              title: t('cleanup.done', {
                count: res.removed.length,
                size: sizeLabel(res.reclaimed_bytes),
              }),
              description:
                res.failed.length > 0
                  ? t('cleanup.partial', { count: res.failed.length })
                  : undefined,
              type: res.failed.length > 0 ? 'warning' : 'success',
            })
            onOpenChange(false)
          },
        })
      }}
    >
      <ul className="max-h-48 overflow-y-auto rounded-lg border border-border text-sm">
        {candidates.map((e) => (
          <li
            key={e.id}
            className="flex items-center justify-between gap-3 border-b border-border px-3 py-1.5 last:border-b-0"
          >
            <span className="truncate font-mono text-xs">
              {imageTitle(e) ?? shortId(e.id)}
            </span>
            <span className="shrink-0 text-xs tabular-nums text-muted-foreground">
              {sizeLabel(e.size_bytes)}
            </span>
          </li>
        ))}
      </ul>
    </ConfirmDialog>
  )
}
