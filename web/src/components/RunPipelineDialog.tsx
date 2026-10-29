import { useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { PlayGlassIcon, TriangleAlertGlassIcon } from '@thesvg/ui-react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useStartPipelineRun } from '../queries/pipelines'
import type { Pipeline } from '../types/pipelines'
import { initialParamValues } from '../lib/pipelineParams'
import { PipelineParamFields } from './PipelineParamFields'

export function RunPipelineDialog({
  appName,
  pipeline,
}: {
  appName: string
  pipeline: Pipeline
}) {
  const { t } = useTranslation('pipelines')
  const params = pipeline.parameters ?? []
  const [open, setOpen] = useState(false)
  const [ref, setRef] = useState('')
  const [sha, setSha] = useState('')
  const [values, setValues] = useState<Record<string, string>>(() =>
    initialParamValues(params),
  )
  const start = useStartPipelineRun(appName)
  const navigate = useNavigate()
  const canRun =
    pipeline.enabled &&
    (pipeline.triggers ?? []).some(
      (trig) => trig === 'manual' || trig === 'api',
    )

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        setOpen(next)
        if (next) {
          setValues(initialParamValues(params))
        } else {
          start.reset()
        }
      }}
    >
      <DialogTrigger
        render={<Button size="sm" disabled={!canRun} />}
        title={canRun ? undefined : t('run.disabledHint')}
      >
        <PlayGlassIcon size="1.2em" aria-hidden="true" />
        {params.length > 0 ? t('run.withParams') : t('run.button')}
      </DialogTrigger>
      <DialogContent className="max-h-[90dvh] overflow-y-auto sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t('run.title', { name: pipeline.name })}</DialogTitle>
          <DialogDescription>{t('run.description')}</DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <div className="space-y-1.5">
            <Label htmlFor="run-ref">{t('run.ref')}</Label>
            <Input
              id="run-ref"
              value={ref}
              onChange={(e) => setRef(e.target.value)}
              placeholder={t('run.refPlaceholder')}
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="run-sha">{t('run.sha')}</Label>
            <Input
              id="run-sha"
              value={sha}
              onChange={(e) => setSha(e.target.value)}
            />
          </div>
          <h3 className="pt-1 text-sm font-semibold text-foreground">
            {t('run.parameters')}
          </h3>
          <PipelineParamFields
            params={params}
            values={values}
            onChange={(name, value) =>
              setValues((prev) => ({ ...prev, [name]: value }))
            }
          />
        </div>
        {start.isError ? (
          <Alert variant="destructive">
            <TriangleAlertGlassIcon
              className="text-tone-warning"
              size="1.2em"
            />
            <AlertDescription>{start.error.message}</AlertDescription>
          </Alert>
        ) : null}
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => setOpen(false)}
          >
            {t('run.cancel')}
          </Button>
          <Button
            type="button"
            disabled={start.isPending}
            onClick={() =>
              start.mutate(
                {
                  name: pipeline.name,
                  req: { ref, sha, inputs: values },
                },
                {
                  onSuccess: (run) => {
                    setOpen(false)
                    void navigate({
                      to: '/apps/$name/pipelines/runs/$runId',
                      params: { name: appName, runId: run.id },
                    })
                  },
                },
              )
            }
          >
            {start.isPending ? t('run.starting') : t('run.start')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
