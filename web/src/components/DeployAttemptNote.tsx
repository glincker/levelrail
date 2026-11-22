import { useTranslation } from 'react-i18next'
import type { DeployAttempt } from '../types/deployAttempt'
import { InfoTip } from './kit'
import { StatusReasonTip } from './StatusReasonTip'
import { deployInterruption } from '../lib/deployInterruption'
import { copyLabelKey, deployCopyOf } from '../lib/deployCopy'

// The reason code a poll-started deploy carries; its label already says why.
const POLLED_REASON = 'DetectedByPoll'

function attemptNote(attempt: DeployAttempt): string | undefined {
  if (attempt.status === 'queued') {
    return `Position ${attempt.queue_position ?? 1}${attempt.wait_reason ? `, ${attempt.wait_reason}` : ''}`
  }
  if (attempt.status === 'held' || attempt.status === 'awaiting_approval') {
    return attempt.wait_reason
  }
  if (attempt.status === 'superseded' && attempt.superseded_by) {
    return `Superseded by ${attempt.superseded_by}`
  }
  if (attempt.status === 'canceled') {
    return `Canceled${attempt.canceled_by ? ` by ${attempt.canceled_by}` : ''}`
  }
  return attempt.reason === POLLED_REASON ? undefined : attempt.reason
}

export function DeployAttemptNote({ attempt }: { attempt: DeployAttempt }) {
  const { t } = useTranslation('common')
  const { t: td } = useTranslation('deploys')
  const kind = deployInterruption(attempt.reason)
  const copy = deployCopyOf(attempt)
  const copyNote = copy
    ? td(copyLabelKey(copy), { id: copy.target })
    : undefined
  const note = kind
    ? td(`interruption.${kind}`)
    : (copyNote ?? attemptNote(attempt))
  if (!note) return null
  return (
    <p className="mt-1.5 flex items-center gap-1 text-xs text-muted-foreground">
      {note}
      {attempt.status === 'queued' ? (
        <InfoTip label={t('deployNote.queueLabel')}>
          {t('deployNote.queueBody')}
        </InfoTip>
      ) : null}
      {kind ? (
        <InfoTip label={td('interruption.retry')}>
          {td('interruption.retry')}
        </InfoTip>
      ) : note === attempt.reason ? (
        <StatusReasonTip code={attempt.reason} />
      ) : null}
    </p>
  )
}
