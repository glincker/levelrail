import { ThumbsDownGlassIcon, ThumbsUpGlassIcon } from '@thesvg/ui-react'
import { useTranslation } from 'react-i18next'
import { Alert } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { useDecidePipelineRunHold } from '../queries/pipelines'
import type { PipelineHold } from '../types/pipelines'

// A run held for approval, such as a pull request from a fork: no job has
// started and no secret has been read until an approver releases it.
export function PipelineHoldGate({
  app,
  runId,
  hold,
}: {
  app: string
  runId: string
  hold: PipelineHold
}) {
  const { t } = useTranslation('pipelines')
  const decide = useDecidePipelineRunHold(app, runId)
  const act = (decision: 'approved' | 'rejected') =>
    decide.mutate(decision, {
      onError: (e) => toast.add({ title: e.message, type: 'error' }),
    })
  return (
    <Alert variant="warning" className="flex flex-wrap items-center gap-2 p-3">
      <div className="min-w-48 flex-1">
        <p className="font-medium text-foreground">{hold.reason}</p>
        <p className="text-xs text-muted-foreground">{t('holdGate.body')}</p>
      </div>
      <Button disabled={decide.isPending} onClick={() => act('approved')}>
        <ThumbsUpGlassIcon size="1.4em" aria-hidden="true" />
        {t('holdGate.approve')}
      </Button>
      <Button
        variant="destructive"
        disabled={decide.isPending}
        onClick={() => act('rejected')}
      >
        <ThumbsDownGlassIcon size="1.4em" aria-hidden="true" />
        {t('holdGate.reject')}
      </Button>
    </Alert>
  )
}
