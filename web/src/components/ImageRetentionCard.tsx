import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'
import { formatBytes } from '../lib/format'
import {
  useImageRetention,
  usePreviewImageRetention,
  useSetImageRetention,
} from '../queries/imageRetention'

// Per-app image keep-last count. Saving is only offered after a dry run.
export function ImageRetentionCard({ appName }: { appName: string }) {
  const { t } = useTranslation('images')
  const inputId = useId()
  const current = useImageRetention(appName)
  const preview = usePreviewImageRetention(appName)
  const save = useSetImageRetention(appName)
  const [draft, setDraft] = useState<string | null>(null)
  const value = draft ?? String(current.data?.keep ?? 0)
  const keep = Number.parseInt(value, 10)
  const min = current.data?.min_keep ?? 2
  const valid = Number.isInteger(keep) && (keep === 0 || keep >= min)
  const previewed = preview.data !== undefined && preview.variables === keep

  const onSave = () => {
    save.mutate(keep, {
      onSuccess: () => {
        setDraft(null)
        preview.reset()
        toast.add({ title: t('retention.saved'), type: 'success' })
      },
      onError: (err) =>
        toast.add({
          title: t('retention.saveFailed'),
          description: err.message,
          type: 'error',
        }),
    })
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('retention.title')}</CardTitle>
        <CardDescription>{t('retention.description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {current.data ? (
          <p className="text-sm text-muted-foreground">
            {t(`retention.source.${current.data.source}`, {
              count: current.data.effective_keep,
              global: current.data.global_keep,
            })}
          </p>
        ) : null}
        {current.data?.protected_environment ? (
          <Alert>
            <AlertDescription>
              {t('retention.protected', {
                env: current.data.protected_environment,
                global: current.data.global_keep,
              })}
            </AlertDescription>
          </Alert>
        ) : null}
        <div className="flex flex-wrap items-end gap-3">
          <div className="space-y-1">
            <label htmlFor={inputId} className="text-sm font-medium">
              {t('retention.keepLabel')}
            </label>
            <Input
              id={inputId}
              type="number"
              inputMode="numeric"
              min={0}
              className="w-28"
              value={value}
              aria-invalid={!valid}
              aria-describedby={`${inputId}-hint`}
              onChange={(e) => {
                setDraft(e.target.value)
                preview.reset()
              }}
            />
          </div>
          <Button
            variant="outline"
            disabled={!valid || preview.isPending}
            onClick={() => preview.mutate(keep)}
          >
            {t('retention.preview')}
          </Button>
          <Button
            disabled={!valid || !previewed || save.isPending}
            onClick={onSave}
          >
            {t('retention.save')}
          </Button>
        </div>
        <p id={`${inputId}-hint`} className="text-xs text-muted-foreground">
          {t('retention.hint', { min })}
        </p>
        {preview.isError ? (
          <p role="alert" className="text-sm text-destructive">
            {preview.error.message}
          </p>
        ) : null}
        {previewed && preview.data ? (
          <div role="status" className="space-y-2">
            <p className="text-sm font-medium">
              {t('retention.wouldRemove', {
                count: preview.data.remove_count,
                size: formatBytes(preview.data.reclaim_bytes),
              })}
            </p>
            {preview.data.remove.length > 0 ? (
              <ul className="max-h-48 space-y-1 overflow-y-auto text-sm text-muted-foreground">
                {preview.data.remove.map((c) => (
                  <li key={c.id} className="flex justify-between gap-3">
                    <span className="truncate font-mono">
                      {c.tags[0] ?? c.id.slice(7, 19)}
                    </span>
                    <span>{formatBytes(c.reclaim_bytes)}</span>
                  </li>
                ))}
              </ul>
            ) : null}
            <p className="text-xs text-muted-foreground">
              {t('retention.alwaysKept')}
            </p>
          </div>
        ) : null}
      </CardContent>
    </Card>
  )
}

export default ImageRetentionCard
