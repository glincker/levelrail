import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CodeGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { useExportIac } from '../queries/iac'
import { IacExportView } from './IacExportView'

type IacExportButtonProps =
  | { kind: 'app' | 'database' | 'node'; name: string; onExported?: undefined }
  | { kind?: undefined; name?: undefined; onExported: (yaml: string) => void }

// One-resource export. Env values stay as placeholders: a per-object copy
// never offers to include them.
export function IacExportButton(props: Readonly<IacExportButtonProps>) {
  const { kind, name, onExported } = props
  const { t } = useTranslation('settings')
  const exportIac = useExportIac()
  const [open, setOpen] = useState(false)

  // Whole live state: hand the text to the caller (the Studio) instead of a dialog.
  if (onExported) {
    return (
      <>
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={exportIac.isPending}
          onClick={() =>
            exportIac.mutate(
              { includeEnvValues: false },
              {
                onSuccess: (r) =>
                  onExported(r.files.map((f) => f.content).join('---\n')),
              },
            )
          }
        >
          <CodeGlassIcon className="size-4" aria-hidden="true" />
          {exportIac.isPending
            ? t('iacExport.running')
            : t('iacExport.studioButton')}
        </Button>
        {exportIac.error ? (
          <span role="alert" className="text-xs text-tone-danger">
            {exportIac.error.message}
          </span>
        ) : null}
      </>
    )
  }

  function openDialog() {
    if (!kind) return
    setOpen(true)
    exportIac.mutate({ [kind]: name, includeEnvValues: false })
  }

  return (
    <>
      <Button type="button" variant="outline" size="sm" onClick={openDialog}>
        <CodeGlassIcon className="size-4" aria-hidden="true" />
        {t('iacExport.button')}
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>{t('iacExport.dialogTitle', { name })}</DialogTitle>
            <DialogDescription>
              {t('iacExport.dialogDescription')}
            </DialogDescription>
          </DialogHeader>
          {exportIac.isPending ? (
            <p className="text-sm text-muted-foreground" role="status">
              {t('iacExport.loading')}
            </p>
          ) : null}
          {exportIac.error ? (
            <p role="alert" className="text-sm text-tone-danger">
              {exportIac.error.message}
            </p>
          ) : null}
          {exportIac.data ? (
            <IacExportView result={exportIac.data} fileName={name} />
          ) : null}
        </DialogContent>
      </Dialog>
    </>
  )
}
