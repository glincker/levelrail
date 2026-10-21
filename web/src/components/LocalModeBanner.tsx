import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ArrowCounterClockwiseIcon,
  LaptopIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { useLocalMode, useResetLocalInstance } from '../queries/localMode'

// Shown on every page of a local instance (the dev command), never on a real server.
export function LocalModeBanner() {
  const { t } = useTranslation('common')
  const { data } = useLocalMode()
  const reset = useResetLocalInstance()
  const [open, setOpen] = useState(false)

  if (!data?.enabled) return null

  return (
    <div
      role="status"
      className="mx-4 mt-2 flex flex-col gap-2 rounded-lg border border-tone-accent-border bg-tone-accent-soft p-3 text-tone-accent sm:flex-row sm:items-center sm:justify-between md:mx-6"
    >
      <div className="flex items-start gap-2">
        <LaptopIcon className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
        <div className="text-sm">
          <p className="font-medium">{t('localMode.title')}</p>
          <p className="text-foreground/80">
            {t('localMode.body')}{' '}
            {data.data_dir ? (
              <code className="break-all text-xs">{data.data_dir}</code>
            ) : null}
          </p>
          {!data.reset_supported && data.reset_hint ? (
            <p className="text-foreground/80">
              {t('localMode.resetUnavailable')}
            </p>
          ) : null}
        </div>
      </div>
      {data.reset_supported ? (
        <Button
          type="button"
          size="sm"
          variant="outline"
          className="shrink-0"
          onClick={() => {
            reset.reset()
            setOpen(true)
          }}
        >
          <ArrowCounterClockwiseIcon className="size-3.5" aria-hidden="true" />
          {t('localMode.reset')}
        </Button>
      ) : null}
      <ConfirmDialog
        open={open}
        onOpenChange={setOpen}
        title={t('localMode.dialog.title')}
        description={t('localMode.dialog.description')}
        consequences={[
          t('localMode.dialog.consequence1'),
          t('localMode.dialog.consequence2'),
        ]}
        confirmLabel={t('localMode.dialog.confirm')}
        pendingLabel={t('localMode.dialog.resetting')}
        cancelLabel={t('localMode.dialog.cancel')}
        tone="destructive"
        requireTyped="reset"
        pending={reset.isPending}
        error={
          reset.error ? reset.error.message || t('localMode.failed') : null
        }
        onConfirm={() =>
          reset.mutate(undefined, { onSuccess: () => setOpen(false) })
        }
      />
    </div>
  )
}
