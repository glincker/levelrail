import { useTranslation } from 'react-i18next'
import { CircleAlertGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { baseJobName } from '../lib/pipelineStatus'
import { firstFailure } from '../lib/pipelineFailure'
import type { PipelineRun } from '../types/pipelines'

export function PipelineFailureSummary({
  run,
  onOpen,
}: {
  run: PipelineRun
  onOpen: (job: string) => void
}) {
  const { t } = useTranslation('pipelines')
  const jobs = run.jobs ?? []
  const failure = run.status === 'failed' ? firstFailure(jobs) : null
  if (!failure) {
    return null
  }
  const skipped = jobs
    .filter(
      (j) =>
        j.status === 'skipped' &&
        (j.reason ?? '').includes(baseJobName(failure.job.key)),
    )
    .map((j) => j.key)
  return (
    <div
      role="alert"
      className="space-y-1 rounded-lg border border-destructive/40 bg-destructive/5 p-3 text-sm"
    >
      <p className="flex flex-wrap items-center gap-1.5 font-medium text-foreground">
        <CircleAlertGlassIcon
          className="size-4.5 text-destructive"
          aria-hidden="true"
        />
        {t('failure.title')}
      </p>
      <p className="text-foreground">
        {t('failure.job', { job: failure.job.key })}
        {failure.stepName
          ? ` ${t('failure.step', { step: failure.stepName })}`
          : ''}
        {failure.code === undefined
          ? ''
          : ` ${t('failure.exit', { code: failure.code })}`}
      </p>
      {failure.job.reason ? (
        <p className="break-words text-muted-foreground">
          {t('failure.reason', { reason: failure.job.reason })}
        </p>
      ) : null}
      {skipped.length > 0 ? (
        <p className="text-muted-foreground">
          {t('failure.skipped', { jobs: skipped.join(', ') })}
        </p>
      ) : null}
      <Button
        variant="outline"
        size="sm"
        onClick={() => onOpen(failure.job.key)}
      >
        {t('failure.open')}
      </Button>
    </div>
  )
}
