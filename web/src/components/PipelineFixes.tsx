import { Link, useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { WrenchGlassIcon } from '@thesvg/ui-react'
import { Button, buttonVariants } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { useRerunPipelineRun } from '../queries/pipelines'
import {
  useApplyPipelineFix,
  usePipelineRunFixes,
  type PipelineFix,
} from '../queries/pipelineUx'
import type { PipelineRun } from '../types/pipelines'

const AUTO_KINDS = ['add_retries', 'raise_timeout', 'raise_memory']

function GuidedFix({
  app,
  pipelineName,
  fix,
  label,
}: {
  app: string
  pipelineName: string
  fix: PipelineFix
  label: string
}) {
  const className = buttonVariants({ variant: 'outline', size: 'sm' })
  if (fix.kind === 'set_secret') {
    return (
      <Link
        to="/apps/$name/environment"
        params={{ name: app }}
        className={className}
      >
        {label}
      </Link>
    )
  }
  if (fix.kind === 'free_disk') {
    return (
      <Link to="/nodes" className={className}>
        {label}
      </Link>
    )
  }
  return (
    <Link
      to="/apps/$name/pipelines/$pipeline"
      params={{ name: app, pipeline: pipelineName }}
      className={className}
    >
      {label}
    </Link>
  )
}

export function PipelineFixes({ app, run }: { app: string; run: PipelineRun }) {
  const { t } = useTranslation('pipelines')
  const navigate = useNavigate()
  const failed = run.status === 'failed'
  const { data: fixes } = usePipelineRunFixes(app, run.id, failed)
  const apply = useApplyPipelineFix(app, run.id)
  const rerun = useRerunPipelineRun(app)
  if (!failed || !fixes || fixes.length === 0) {
    return null
  }
  const openRun = (id: string) =>
    void navigate({
      to: '/apps/$name/pipelines/runs/$runId',
      params: { name: app, runId: id },
    })
  const onAuto = (fix: PipelineFix) =>
    apply.mutate(
      {
        kind: fix.kind,
        job: fix.job,
        rerun: true,
      },
      {
        onSuccess: (res) => {
          toast.add({
            title: res.run ? t('fixes.startedRun') : t('fixes.applied'),
            type: 'success',
          })
          if (res.run) {
            openRun(res.run)
          }
        },
        onError: (e) => toast.add({ title: e.message, type: 'error' }),
      },
    )
  const onRerun = () =>
    rerun.mutate(run.id, {
      onSuccess: (next) => openRun(next.id),
      onError: (e) => toast.add({ title: e.message, type: 'error' }),
    })
  return (
    <div className="space-y-2 rounded-lg border border-border bg-muted/30 p-3 text-sm">
      <p className="flex items-center gap-1.5 font-medium text-foreground">
        <WrenchGlassIcon
          className="size-5 text-muted-foreground"
          aria-hidden="true"
        />
        {t('fixes.title')}
      </p>
      <ul className="space-y-2">
        {fixes.map((fix) => {
          const label = t(`fixes.kind.${fix.kind}`)
          return (
            <li
              key={`${fix.job}/${fix.code}/${fix.kind}`}
              className="flex flex-wrap items-center gap-2"
            >
              <div className="min-w-48 flex-1">
                <p className="text-foreground">
                  {t(`fixes.code.${fix.code}`, { defaultValue: fix.hint })}
                </p>
                <p
                  className="truncate text-xs text-muted-foreground"
                  title={fix.match}
                >
                  {t('fixes.job', { job: fix.job })}: {fix.match}
                </p>
              </div>
              {AUTO_KINDS.includes(fix.kind) ? (
                <Button
                  size="sm"
                  disabled={apply.isPending}
                  onClick={() => onAuto(fix)}
                >
                  {label}
                </Button>
              ) : fix.kind === 'rerun' ? (
                <Button size="sm" disabled={rerun.isPending} onClick={onRerun}>
                  {label}
                </Button>
              ) : (
                <GuidedFix
                  app={app}
                  pipelineName={run.pipeline_name}
                  fix={fix}
                  label={label}
                />
              )}
            </li>
          )
        })}
      </ul>
    </div>
  )
}
