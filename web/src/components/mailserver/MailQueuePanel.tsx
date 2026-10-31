import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { PauseGlassIcon, PlayGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { useCanWriteSensitive } from '../../hooks/useCanWriteSensitive'
import { useStepUpGate } from '../../hooks/useStepUpGate'
import {
  isHiddenQueueError,
  mailStepUpScope,
  setQueueHold,
  useInvalidateMail,
  useMailQueue,
} from '../../queries/mailMailboxes'
import { StepUpDialog } from '../env/StepUpDialog'
import { PanelCard, Reason } from './mailPanelUi'

/** Instance-wide queue. Hidden when the API refuses it for this caller. */
export function MailQueuePanel() {
  const { t } = useTranslation('mailServer')
  const q = useMailQueue()
  const canWrite = useCanWriteSensitive()
  const invalidate = useInvalidateMail()
  const [error, setError] = useState<string | null>(null)
  const onError = useCallback(
    (e: unknown) =>
      setError(e instanceof Error ? e.message : t('mailboxes.failed')),
    [t],
  )
  const gate = useStepUpGate(onError)
  if (q.isError && isHiddenQueueError(q.error)) return null
  if (!q.data) return null
  const { app, entries } = q.data
  const why = canWrite ? undefined : t('mailboxes.needsWrite')

  const act = (hold: boolean, ids: number[]) => {
    setError(null)
    void gate.guard(mailStepUpScope.release(app), async () => {
      await setQueueHold(hold, app, ids)
      void invalidate()
    })
  }
  return (
    <PanelCard label={t('queue.title')} title={t('queue.title')}>
      <div className="flex flex-wrap items-center gap-2 text-xs">
        <span className="text-muted-foreground">
          {t('queue.count', { count: entries.length })}
        </span>
        <Reason why={why}>
          <Button
            size="sm"
            variant="outline"
            className="gap-1"
            disabled={!canWrite || entries.length === 0}
            onClick={() => act(true, [])}
          >
            <PauseGlassIcon size="1.1em" />
            {t('queue.holdAll')}
          </Button>
        </Reason>
        <Reason why={why}>
          <Button
            size="sm"
            variant="outline"
            className="gap-1"
            disabled={!canWrite || entries.length === 0}
            onClick={() => act(false, [])}
          >
            <PlayGlassIcon size="1.1em" />
            {t('queue.releaseAll')}
          </Button>
        </Reason>
      </div>
      {error && (
        <p role="alert" className="text-xs text-tone-danger">
          {error}
        </p>
      )}
      {entries.length === 0 ? (
        <p className="text-xs text-muted-foreground">{t('queue.empty')}</p>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-left text-xs">
            <thead className="text-muted-foreground">
              <tr>
                <th className="py-1 pr-3 font-medium">
                  {t('queue.col.sender')}
                </th>
                <th className="py-1 pr-3 font-medium">
                  {t('queue.col.domain')}
                </th>
                <th className="py-1 pr-3 font-medium">
                  {t('queue.col.attempts')}
                </th>
                <th className="py-1 pr-3 font-medium">{t('queue.col.next')}</th>
                <th className="py-1 pr-3 font-medium">
                  {t('queue.col.error')}
                </th>
                <th className="py-1 font-medium">
                  <span className="sr-only">{t('mailboxes.col.actions')}</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {entries.map((m) => (
                <tr key={m.id} className="border-t border-border align-top">
                  <td className="py-1.5 pr-3 font-mono">{m.sender_account}</td>
                  <td className="py-1.5 pr-3">{m.recipient_domain}</td>
                  <td className="py-1.5 pr-3">{m.attempts}</td>
                  <td className="py-1.5 pr-3 whitespace-nowrap">
                    {m.hold
                      ? t('queue.held')
                      : new Date(m.next_attempt).toLocaleString()}
                  </td>
                  <td className="py-1.5 pr-3 break-words">
                    {m.last_error ?? ''}
                  </td>
                  <td className="py-1.5 text-right">
                    <Reason why={why}>
                      <Button
                        size="sm"
                        variant="outline"
                        disabled={!canWrite}
                        aria-label={t(
                          m.hold ? 'queue.releaseFor' : 'queue.holdFor',
                          { id: m.id },
                        )}
                        onClick={() => act(!m.hold, [m.id])}
                      >
                        {m.hold ? t('queue.release') : t('queue.hold')}
                      </Button>
                    </Reason>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      <StepUpDialog
        scope={gate.pending?.scope ?? null}
        onVerified={() => {
          void gate.onVerified()
        }}
        onCancel={gate.cancel}
      />
    </PanelCard>
  )
}
