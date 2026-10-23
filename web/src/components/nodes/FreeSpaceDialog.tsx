import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { BroomIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { toast } from '@/components/ui/toast'
import { formatBytes } from '../../lib/format'
import { useCleanupPreview, useRunCleanup } from '../../queries/nodeCapacity'
import type { CleanupClass } from '../../types/nodeCapacity'
import { CLASS_HINT, CLASS_TITLE } from './capacityKeys'

function ClassRow({ cls }: Readonly<{ cls: CleanupClass }>) {
  const { t } = useTranslation('nodes')
  return (
    <li className="space-y-1 rounded-lg border p-3">
      <div className="flex items-center justify-between gap-2">
        <span className="text-sm font-medium">{t(CLASS_TITLE[cls.name])}</span>
        <span className="flex items-center gap-2 text-sm">
          {cls.deletable ? null : (
            <Badge variant="muted">{t('capacity.neverRemoved')}</Badge>
          )}
          {formatBytes(cls.bytes)}
        </span>
      </div>
      <p className="text-xs text-muted-foreground">{t(CLASS_HINT[cls.name])}</p>
      {cls.items.length > 0 ? (
        <p className="truncate text-xs text-muted-foreground">
          {cls.items
            .slice(0, 5)
            .map((i) => i.name)
            .join(', ')}
          {cls.items.length > 5
            ? ` ${t('capacity.more', { count: cls.items.length - 5 })}`
            : ''}
        </p>
      ) : null}
    </li>
  )
}

export function FreeSpaceDialog({ nodeId }: Readonly<{ nodeId: string }>) {
  const { t } = useTranslation('nodes')
  const [open, setOpen] = useState(false)
  const preview = useCleanupPreview(nodeId)
  const run = useRunCleanup(nodeId)
  const classes = preview.data?.preview.classes ?? []
  const total = preview.data?.preview.total_bytes ?? 0

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (next) {
      preview.mutate()
    } else {
      preview.reset()
      run.reset()
    }
  }

  return (
    <>
      <Button variant="outline" onClick={() => handleOpenChange(true)}>
        <BroomIcon className="size-4" />
        {t('capacity.freeSpace')}
      </Button>
      <Dialog open={open} onOpenChange={handleOpenChange}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('capacity.dialogTitle')}</DialogTitle>
            <DialogDescription>{t('capacity.dialogBody')}</DialogDescription>
          </DialogHeader>
          {preview.isPending ? (
            <p className="text-sm text-muted-foreground">
              {t('capacity.scanning')}
            </p>
          ) : null}
          {preview.isError ? (
            <Alert variant="destructive">
              <AlertDescription>{preview.error.message}</AlertDescription>
            </Alert>
          ) : null}
          {classes.length > 0 ? (
            <ul className="space-y-2">
              {classes.map((c) => (
                <ClassRow key={c.name} cls={c} />
              ))}
            </ul>
          ) : null}
          {run.isError ? (
            <Alert variant="destructive">
              <AlertDescription>{run.error.message}</AlertDescription>
            </Alert>
          ) : null}
          <DialogFooter>
            <Button variant="ghost" onClick={() => handleOpenChange(false)}>
              {t('capacity.cancel')}
            </Button>
            <Button
              disabled={!preview.isSuccess || total <= 0 || run.isPending}
              onClick={() =>
                run.mutate(undefined, {
                  onSuccess: (res) => {
                    handleOpenChange(false)
                    toast.add({
                      title: t('capacity.freed', {
                        size: formatBytes(res.freed_bytes),
                      }),
                      type: 'success',
                    })
                  },
                })
              }
            >
              {run.isPending
                ? t('capacity.freeing')
                : t('capacity.confirm', { size: formatBytes(total) })}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
