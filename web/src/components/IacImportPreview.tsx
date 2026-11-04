import { useRef, useState } from 'react'
import type { ChangeEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { CodeGlassIcon, UploadGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { IacIssuesError, usePlanIac } from '../queries/iac'
import { IacPlanView } from './IacPlanView'

interface IacImportPreviewProps {
  name: string
}

// Preview only: the plan call never changes state, apply stays in the CLI.
export function IacImportPreview({ name }: IacImportPreviewProps) {
  const { t } = useTranslation('settings')
  const planIac = usePlanIac()
  const [open, setOpen] = useState(false)
  const [text, setText] = useState('')
  const [fileName, setFileName] = useState('pasted.yaml')
  const uploadRef = useRef<HTMLInputElement>(null)
  const error = planIac.error

  function runPreview() {
    planIac.mutate({ files: [{ name: fileName, content: text }] })
  }

  async function onUpload(e: ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    e.target.value = ''
    if (!file) return
    setText(await file.text())
    setFileName(file.name)
    planIac.reset()
  }

  return (
    <>
      <Button
        type="button"
        variant="outline"
        size="sm"
        onClick={() => setOpen(true)}
      >
        <UploadGlassIcon className="size-4" aria-hidden="true" />
        {t('iacImport.button')}
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>{t('iacImport.dialogTitle', { name })}</DialogTitle>
            <DialogDescription>
              {t('iacImport.dialogDescription')}
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-2">
            <Label htmlFor="iac-import-yaml">{t('iacImport.textLabel')}</Label>
            <Textarea
              id="iac-import-yaml"
              className="min-h-32 font-mono text-xs"
              value={text}
              placeholder={t('iacImport.placeholder')}
              spellCheck={false}
              onChange={(e) => {
                setText(e.target.value)
                setFileName('pasted.yaml')
              }}
            />
            <div className="flex flex-wrap items-center gap-2">
              <Button
                type="button"
                size="sm"
                disabled={!text.trim() || planIac.isPending}
                onClick={runPreview}
              >
                <CodeGlassIcon className="size-4" aria-hidden="true" />
                {planIac.isPending
                  ? t('iacImport.previewing')
                  : t('iacImport.preview')}
              </Button>
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => uploadRef.current?.click()}
              >
                <UploadGlassIcon className="size-4" aria-hidden="true" />
                {t('iacImport.upload')}
              </Button>
              <input
                ref={uploadRef}
                type="file"
                accept=".yaml,.yml,text/yaml"
                className="sr-only"
                aria-label={t('iacImport.uploadLabel')}
                tabIndex={-1}
                onChange={(e) => void onUpload(e)}
              />
            </div>
          </div>
          <div aria-live="polite" className="space-y-2">
            {error instanceof IacIssuesError ? (
              <ul
                role="alert"
                className="space-y-1 font-mono text-sm text-red-700 dark:text-red-400"
              >
                {error.issues.map((i) => (
                  <li key={`${i.file}:${i.line}:${i.message}`}>
                    {i.file}:{i.line}: {i.message}
                    {i.path ? ` (${i.path})` : ''}
                  </li>
                ))}
              </ul>
            ) : error ? (
              <p
                role="alert"
                className="text-sm text-red-700 dark:text-red-400"
              >
                {error.message}
              </p>
            ) : planIac.data ? (
              <IacPlanView plan={planIac.data} />
            ) : null}
          </div>
          <p className="text-xs text-muted-foreground">
            {t('iacImport.cliNote')}{' '}
            <code className="rounded bg-muted px-1 py-0.5 font-mono">
              levelrail-cli apply -f {fileName}
            </code>
          </p>
        </DialogContent>
      </Dialog>
    </>
  )
}
