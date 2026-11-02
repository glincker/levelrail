import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { DownloadGlassIcon, TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { StepUpDialog } from '../env/StepUpDialog'
import { isStepUpRequired } from '../../queries/envProtection'
import { useProjectExport } from '../../queries/projectExport'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { CommandBlock } from '@/components/ui/code-block'
import {
  Collapsible,
  CollapsiblePanel,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'

function saveBlob(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  a.remove()
  URL.revokeObjectURL(url)
}

// Secret values need a step-up grant per app, so the dialog walks the
// project's apps in order and retries until the server lets the export through.
export function ExportProjectDialog({
  projectId,
  projectName,
  appNames,
}: {
  projectId: string
  projectName: string
  appNames: string[]
}) {
  const { t } = useTranslation('projectExport')
  const [open, setOpen] = useState(false)
  const [includeSecrets, setIncludeSecrets] = useState(false)
  const [stepIndex, setStepIndex] = useState<number | null>(null)
  const [localError, setLocalError] = useState<string | null>(null)
  const exportMutation = useProjectExport()

  const reset = () => {
    setIncludeSecrets(false)
    setStepIndex(null)
    setLocalError(null)
    exportMutation.reset()
  }

  const run = (nextStep: number | null) => {
    setLocalError(null)
    exportMutation.mutate(
      { project: projectId, includeSecrets },
      {
        onSuccess: ({ blob, filename }) => {
          setStepIndex(null)
          saveBlob(blob, filename)
          toast.add({ title: t('done'), type: 'success' })
          setOpen(false)
        },
        onError: (error) => {
          if (!isStepUpRequired(error)) {
            setStepIndex(null)
            return
          }
          const next = nextStep ?? 0
          if (next >= appNames.length) {
            setStepIndex(null)
            setLocalError(t('errors.stepUpExhausted'))
            return
          }
          setStepIndex(next)
        },
      },
    )
  }

  const stepApp = stepIndex === null ? undefined : appNames[stepIndex]
  const errorMessage =
    localError ??
    (exportMutation.isError && !isStepUpRequired(exportMutation.error)
      ? exportMutation.error.message
      : null)
  const cli = `levelrail-cli apps projects export ${projectName} -o ./${projectName}${
    includeSecrets ? ' --include-secrets' : ''
  }`

  return (
    <>
      <Dialog
        open={open}
        onOpenChange={(next) => {
          setOpen(next)
          if (!next) reset()
        }}
      >
        <DialogTrigger
          render={
            <Button
              variant="outline"
              size="sm"
              disabled={appNames.length === 0}
            />
          }
        >
          <DownloadGlassIcon size="1.4em" aria-hidden="true" />
          {t('trigger')}
        </DialogTrigger>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{t('title')}</DialogTitle>
            <DialogDescription>{t('description')}</DialogDescription>
          </DialogHeader>
          <div className="space-y-1 text-sm text-muted-foreground">
            <p>{t('includes')}</p>
            <p>{t('gaps')}</p>
          </div>
          <div className="flex items-start justify-between gap-3 rounded-lg border border-border px-3 py-2">
            <div className="min-w-0">
              <label
                htmlFor="project-export-secrets"
                className="text-sm font-medium"
              >
                {t('secrets.title')}
              </label>
              <p className="text-xs text-muted-foreground">
                {t('secrets.description')}
              </p>
            </div>
            <Switch
              id="project-export-secrets"
              size="sm"
              checked={includeSecrets}
              onCheckedChange={setIncludeSecrets}
            />
          </div>
          {includeSecrets ? (
            <Alert>
              <TriangleAlertGlassIcon
                className="size-4 text-tone-warning"
                aria-hidden="true"
              />
              <AlertDescription>{t('secrets.warning')}</AlertDescription>
            </Alert>
          ) : null}
          {errorMessage ? (
            <p role="alert" className="text-sm text-destructive">
              {errorMessage}
            </p>
          ) : null}
          <Collapsible>
            <CollapsibleTrigger className="text-xs text-muted-foreground hover:text-foreground">
              {t('cli.summary')}
            </CollapsibleTrigger>
            <CollapsiblePanel className="space-y-1 pt-2">
              <p className="text-xs text-muted-foreground">{t('cli.hint')}</p>
              <CommandBlock code={cli} wrap />
            </CollapsiblePanel>
          </Collapsible>
          <DialogFooter>
            <Button variant="outline" onClick={() => setOpen(false)}>
              {t('cancel')}
            </Button>
            <Button
              disabled={exportMutation.isPending}
              onClick={() => run(null)}
            >
              <DownloadGlassIcon size="1.4em" aria-hidden="true" />
              {exportMutation.isPending ? t('downloading') : t('download')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <StepUpDialog
        scope={stepApp ? { action: 'export', app: stepApp } : null}
        subject={stepApp}
        description={stepApp ? t('stepUp', { app: stepApp }) : undefined}
        onVerified={() => run((stepIndex ?? 0) + 1)}
        onCancel={() => setStepIndex(null)}
      />
    </>
  )
}
