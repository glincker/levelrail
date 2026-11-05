import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CircleAlertGlassIcon, TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import {
  useImageRemove,
  type ImageGCResult,
} from '../../queries/imageInventory'
import { imageTitle, shortId, sizeLabel } from './imageFormat'

interface Props {
  node: string
  open: boolean
  onOpenChange: (open: boolean) => void
  // Explicit images to remove; empty means "everything the toggle selects".
  ids: string[]
  // Offers the "other images no container uses" switch (cleanup flow only).
  allowUnmanaged: boolean
  onRemoved: (ids: string[]) => void
}

export function ImageRemoveDialog(props: Props) {
  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        {/* Mounted only while open, so every open starts from a fresh preview. */}
        <RemoveBody {...props} />
      </DialogContent>
    </Dialog>
  )
}

function RemoveBody({
  node,
  onOpenChange,
  ids,
  allowUnmanaged,
  onRemoved,
}: Props) {
  const { t } = useTranslation('images')
  const remove = useImageRemove(node)
  const { mutate } = remove
  const [includeUnmanaged, setIncludeUnmanaged] = useState(false)
  const [result, setResult] = useState<ImageGCResult | null>(null)
  const [preview, setPreview] = useState<ImageGCResult | null>(null)
  const [previewError, setPreviewError] = useState<string | null>(null)
  const [running, setRunning] = useState(false)
  const idsKey = ids.join(',')

  // Re-preview whenever the dialog opens or the scope changes.
  useEffect(() => {
    if (ids.length === 0 && !includeUnmanaged) return
    mutate(
      { ids, dryRun: true, includeUnmanagedUnused: includeUnmanaged },
      {
        onSuccess: (res) => {
          setPreview(res)
          setPreviewError(null)
        },
        onError: (e) => {
          setPreview(null)
          setPreviewError(e.message)
        },
      },
    )
    // ids is tracked through idsKey so a new array with the same content does not refire.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [idsKey, includeUnmanaged, mutate])

  const candidates = preview?.candidates ?? []
  const nameOf = (id: string): string => {
    const c = candidates.find((x) => x.id === id)
    return (c ? imageTitle(c) : null) ?? shortId(id)
  }
  const previewing = remove.isPending && !running
  const total = candidates.reduce((s, c) => s + c.size_bytes, 0)

  const run = () => {
    setRunning(true)
    mutate(
      { ids, dryRun: false, includeUnmanagedUnused: includeUnmanaged },
      {
        onSuccess: (res) => {
          setResult(res)
          setRunning(false)
          onRemoved(res.removed)
        },
        onError: (e) => {
          setPreviewError(e.message)
          setRunning(false)
        },
      },
    )
  }

  return (
    <>
      <DialogHeader>
        <DialogTitle>{t('remove.title')}</DialogTitle>
        <DialogDescription>{t('remove.description')}</DialogDescription>
      </DialogHeader>

      <div aria-live="polite" className="flex flex-col gap-3 text-sm">
        {result ? (
          <div className="flex flex-col gap-2">
            <p className="font-medium">
              {t('remove.resultRemoved', {
                count: result.removed.length,
                size: sizeLabel(result.reclaimed_bytes),
              })}
            </p>
            {result.failed.length > 0 ? (
              <Alert variant="destructive">
                <AlertDescription>
                  <p className="font-medium">
                    {t('remove.resultFailed', {
                      count: result.failed.length,
                    })}
                  </p>
                  <ul className="mt-1 max-h-40 overflow-y-auto">
                    {result.failed.map((f) => (
                      <li key={f.id} className="break-words">
                        <span className="font-mono text-xs">
                          {nameOf(f.id)}
                        </span>
                        : {f.error}
                      </li>
                    ))}
                  </ul>
                </AlertDescription>
              </Alert>
            ) : null}
          </div>
        ) : (
          <>
            {allowUnmanaged ? (
              <div className="flex items-center gap-2">
                <Switch
                  id="remove-include-unmanaged"
                  checked={includeUnmanaged}
                  disabled={running}
                  onCheckedChange={setIncludeUnmanaged}
                />
                <Label htmlFor="remove-include-unmanaged">
                  {t('remove.includeUnmanaged')}
                </Label>
              </div>
            ) : null}
            {previewing ? (
              <p className="text-muted-foreground">{t('remove.previewing')}</p>
            ) : null}
            {previewError ? (
              <Alert variant="destructive">
                <AlertDescription>{previewError}</AlertDescription>
              </Alert>
            ) : null}
            {preview && candidates.length === 0 ? (
              <p className="text-muted-foreground">{t('remove.nothing')}</p>
            ) : null}
            {candidates.length > 0 ? (
              <>
                <ul
                  aria-label={t('remove.listLabel')}
                  className="max-h-48 overflow-y-auto rounded-lg border border-border"
                >
                  {candidates.map((c) => (
                    <li
                      key={c.id}
                      className="flex items-center justify-between gap-3 border-b border-border px-3 py-1.5 last:border-b-0"
                    >
                      <span className="truncate font-mono text-xs">
                        {imageTitle(c) ?? shortId(c.id)}
                      </span>
                      <span className="shrink-0 text-xs tabular-nums text-muted-foreground">
                        {sizeLabel(c.size_bytes)}
                      </span>
                    </li>
                  ))}
                </ul>
                <p className="flex items-center gap-1.5 font-medium">
                  <CircleAlertGlassIcon size="1.2em" aria-hidden="true" />
                  {t('remove.total', {
                    count: candidates.length,
                    size: sizeLabel(preview?.reclaimed_bytes ?? total),
                  })}
                </p>
                <p className="flex items-start gap-1.5 text-muted-foreground">
                  <TriangleAlertGlassIcon
                    size="1.2em"
                    className="mt-0.5 shrink-0 text-tone-warning"
                    aria-hidden="true"
                  />
                  {t('remove.warning')}
                </p>
              </>
            ) : null}
          </>
        )}
      </div>

      <DialogFooter>
        {result ? (
          <Button
            onClick={() => {
              onOpenChange(false)
            }}
          >
            {t('remove.done')}
          </Button>
        ) : (
          <>
            <Button
              variant="outline"
              disabled={running}
              onClick={() => {
                onOpenChange(false)
              }}
            >
              {t('remove.cancel')}
            </Button>
            <Button
              variant="destructive"
              disabled={candidates.length === 0 || previewing || running}
              onClick={run}
            >
              {running
                ? t('remove.removing')
                : t('remove.confirm', { count: candidates.length })}
            </Button>
          </>
        )}
      </DialogFooter>
    </>
  )
}
