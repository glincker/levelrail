import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Field, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { useComposeDbRestore } from '../queries/composeDbBackups'

// Mounted only while open, so the read-only preview fires once per open. The
// confirm phrase always comes from the server, never built here.
export function ComposeDbRestoreDialog({
  app,
  service,
  run,
  onClose,
}: {
  app: string
  service: string
  run: string
  onClose: () => void
}) {
  const { t } = useTranslation('backups')
  const preview = useComposeDbRestore(app, service)
  const confirm = useComposeDbRestore(app, service)
  const [typed, setTyped] = useState('')
  const { mutate: runPreview } = preview

  useEffect(() => {
    runPreview({ run, confirm: '', preview: true })
  }, [runPreview, run])

  const info = preview.data
  const phrase = info?.confirm_phrase ?? ''
  const started = confirm.data
  const error = confirm.error ?? preview.error

  return (
    <Dialog
      open
      onOpenChange={(next) => {
        if (!next) onClose()
      }}
    >
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t('composeDb.restoreTitle', { service })}</DialogTitle>
          <DialogDescription>{t('composeDb.restoreWarning')}</DialogDescription>
        </DialogHeader>
        {info ? (
          <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-sm">
            <dt className="text-muted-foreground">
              {t('composeDb.restoreEngine')}
            </dt>
            <dd>{info.engine}</dd>
            <dt className="text-muted-foreground">
              {t('composeDb.restoreObject')}
            </dt>
            <dd className="min-w-0 break-all font-mono text-xs">
              {info.object_key}
            </dd>
            <dt className="text-muted-foreground">
              {t('composeDb.restoreSize')}
            </dt>
            <dd className="tabular-nums">{info.size_bytes} B</dd>
          </dl>
        ) : preview.isPending ? (
          <p className="text-sm text-muted-foreground">
            {t('composeDb.restoreLoading')}
          </p>
        ) : null}
        {info && !started ? (
          <Field>
            <FieldLabel htmlFor="compose-db-restore-phrase">
              {t('composeDb.restorePhraseLabel', { phrase })}
            </FieldLabel>
            <Input
              id="compose-db-restore-phrase"
              autoComplete="off"
              spellCheck={false}
              value={typed}
              onChange={(event) => {
                setTyped(event.target.value)
              }}
            />
          </Field>
        ) : null}
        {started?.safety_run_id ? (
          <p role="status" className="text-sm">
            {t('composeDb.restoreAccepted', { id: started.safety_run_id })}
          </p>
        ) : null}
        {error ? (
          <p role="alert" className="text-sm text-destructive">
            {error.message}
          </p>
        ) : null}
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            {started
              ? t('composeDb.restoreClose')
              : t('composeDb.restoreCancel')}
          </Button>
          {!started ? (
            <Button
              type="button"
              variant="destructive"
              disabled={
                !info || phrase === '' || typed !== phrase || confirm.isPending
              }
              onClick={() => {
                confirm.mutate({ run, confirm: phrase, preview: false })
              }}
            >
              {confirm.isPending
                ? t('composeDb.restoreStarting')
                : t('composeDb.restoreConfirm')}
            </Button>
          ) : null}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
