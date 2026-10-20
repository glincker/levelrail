import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQueryClient } from '@tanstack/react-query'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import {
  goLiveKeys,
  useApplyChecklistAction,
  useChecklistStream,
} from '../../queries/goLiveChecklist'
import type {
  ChecklistAction,
  ChecklistStep,
  GoLiveChecklistData,
} from '../../queries/goLiveChecklist'
import { FirewallFixDialog } from './FirewallFixDialog'
import { STATE_VARIANT } from './liveState'
import { StepRow } from './StepRow'
import { Timeline } from './Timeline'

/** useStatusAnnouncements returns a sentence for each step whose status changed since the last snapshot. */
function useStatusAnnouncement(cl: GoLiveChecklistData | undefined): string {
  const { t } = useTranslation('golive')
  const prev = useRef<Map<string, string>>(new Map())
  const [message, setMessage] = useState('')
  useEffect(() => {
    if (!cl) return
    const changed = cl.steps.filter(
      (s) => prev.current.has(s.id) && prev.current.get(s.id) !== s.status,
    )
    if (changed.length > 0) {
      setMessage(
        changed
          .map((s) =>
            t('announce', { title: s.title, status: t(`status.${s.status}`) }),
          )
          .join('. '),
      )
    }
    prev.current = new Map(cl.steps.map((s) => [s.id, s.status]))
  }, [cl, t])
  return message
}

export function GoLiveChecklist({
  app,
  domain,
}: {
  app: string
  domain: string
}) {
  const { t } = useTranslation('golive')
  const queryClient = useQueryClient()
  const { checklist, error, refresh } = useChecklistStream(app, domain)
  const announcement = useStatusAnnouncement(checklist)
  const [confirm, setConfirm] = useState<ChecklistStep>()
  const apply = useApplyChecklistAction(() => {
    setConfirm(undefined)
    void queryClient.invalidateQueries({ queryKey: goLiveKeys.status })
    refresh()
  })

  function onAction(action: ChecklistAction, step: ChecklistStep) {
    if (action.kind === 'recheck') {
      refresh()
      return
    }
    if (action.kind === 'fix' && step.firewall_fix) {
      setConfirm(step)
      return
    }
    apply.mutate(action)
  }

  if (error && !checklist) {
    return (
      <Alert variant="destructive">
        <AlertDescription className="space-y-2">
          <p>{t('loadFailed', { message: error.message })}</p>
          <Button type="button" size="sm" variant="outline" onClick={refresh}>
            {t('retry')}
          </Button>
        </AlertDescription>
      </Alert>
    )
  }
  if (!checklist) {
    return (
      <div role="status" aria-label={t('loading')} className="space-y-3">
        <Skeleton className="h-20 w-full" />
        <Skeleton className="h-20 w-full" />
        <Skeleton className="h-20 w-full" />
      </div>
    )
  }
  const fixStep = confirm
  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-2">
        <Badge variant={STATE_VARIANT[checklist.state]}>
          {t(`state.${checklist.state}`)}
        </Badge>
        {checklist.live && checklist.url ? (
          <Button
            size="sm"
            nativeButton={false}
            render={<a href={checklist.url} target="_blank" rel="noreferrer" />}
          >
            {t('openSite', { url: checklist.url.replace(/^https?:\/\//, '') })}
          </Button>
        ) : null}
        <Button type="button" size="sm" variant="outline" onClick={refresh}>
          {t('recheck')}
        </Button>
      </div>
      <div role="status" aria-live="polite" className="sr-only">
        {announcement}
      </div>
      {apply.error ? (
        <Alert variant="destructive">
          <AlertDescription>
            {t('actionFailed', { message: apply.error.message })}
          </AlertDescription>
        </Alert>
      ) : null}
      <ol className="space-y-3">
        {checklist.steps.map((s, i) => (
          <StepRow
            key={s.id}
            step={s}
            index={i}
            pending={apply.isPending}
            onAction={onAction}
          />
        ))}
      </ol>
      <Timeline events={checklist.timeline} />
      {fixStep?.firewall_fix ? (
        <FirewallFixDialog
          fix={fixStep.firewall_fix}
          open
          pending={apply.isPending}
          error={apply.error?.message ?? null}
          onOpenChange={(o) => {
            if (!o) setConfirm(undefined)
          }}
          onConfirm={() => {
            if (fixStep.primary) apply.mutate(fixStep.primary)
          }}
        />
      ) : null}
    </div>
  )
}
