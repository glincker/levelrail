import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Checkbox } from '@/components/ui/checkbox'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { toast } from '@/components/ui/toast'
import { ApiError } from '../../lib/apiError'
import {
  buildQueueKillAllStepUp,
  useKillAllBuilds,
  type BuildQueue,
} from '../../queries/buildQueue'
import { StepUpDialog } from '../env/StepUpDialog'

export const KILL_CONFIRM_WORD = 'KILL'
const NAMES_SHOWN = 8

function appNames(queue: BuildQueue): string[] {
  const seen = new Set<string>()
  for (const r of queue.running) seen.add(r.app)
  for (const q of queue.queued) seen.add(q.app)
  return [...seen]
}

export function KillAllDialog({
  open,
  onOpenChange,
  queue,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  queue: BuildQueue
}) {
  const { t } = useTranslation('deploys')
  const kill = useKillAllBuilds()
  const [pause, setPause] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [stepUp, setStepUp] = useState(false)
  const names = appNames(queue)
  const extra = names.length - NAMES_SHOWN

  const close = () => {
    setPause(false)
    setError(null)
    onOpenChange(false)
  }

  const run = () => {
    setError(null)
    kill.mutate(
      { pause },
      {
        onSuccess: (res) => {
          toast.add({
            title: t('buildQueue.kill.done', { count: res.canceled.length }),
            description:
              res.failed.length > 0
                ? t('buildQueue.kill.someFailed', {
                    count: res.failed.length,
                    apps: res.failed.map((f) => f.app).join(', '),
                  })
                : undefined,
            type: res.failed.length > 0 ? 'warning' : 'success',
          })
          close()
        },
        onError: (err) => {
          if (err instanceof ApiError && err.code === 'step_up_required') {
            setStepUp(true)
            return
          }
          setError(
            err instanceof ApiError && err.status === 409
              ? t('buildQueue.kill.conflict')
              : err.message || t('buildQueue.kill.failed'),
          )
        },
      },
    )
  }

  return (
    <>
      <ConfirmDialog
        open={open}
        onOpenChange={(next) => {
          if (next) onOpenChange(true)
          else close()
        }}
        tone="destructive"
        solidConfirm
        focusCancel
        title={t('buildQueue.kill.title')}
        description={t('buildQueue.kill.description', {
          running: queue.running.length,
          queued: queue.queued.length,
        })}
        confirmLabel={t('buildQueue.kill.confirm')}
        pendingLabel={t('buildQueue.kill.confirming')}
        pending={kill.isPending}
        requireTyped={KILL_CONFIRM_WORD}
        error={error}
        onConfirm={run}
      >
        <div className="space-y-3 text-sm">
          {names.length > 0 ? (
            <p className="text-muted-foreground">
              {names.slice(0, NAMES_SHOWN).join(', ')}
              {extra > 0
                ? ` ${t('buildQueue.kill.more', { count: extra })}`
                : ''}
            </p>
          ) : (
            <p className="text-muted-foreground">{t('buildQueue.kill.none')}</p>
          )}
          <label className="flex items-center gap-2">
            <Checkbox
              checked={pause}
              onCheckedChange={(v) => {
                setPause(v === true)
              }}
              aria-label={t('buildQueue.kill.alsoPause')}
            />
            <span>{t('buildQueue.kill.alsoPause')}</span>
          </label>
        </div>
      </ConfirmDialog>
      <StepUpDialog
        scope={stepUp ? buildQueueKillAllStepUp : null}
        description={t('buildQueue.kill.stepUp')}
        onVerified={() => {
          setStepUp(false)
          run()
        }}
        onCancel={() => {
          setStepUp(false)
        }}
      />
    </>
  )
}
