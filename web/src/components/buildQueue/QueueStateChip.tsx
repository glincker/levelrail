import { useTranslation } from 'react-i18next'
import { StatusPill } from '@/components/kit'
import type { Tone } from '@/components/kit'
import type { BuildQueue } from '../../queries/buildQueue'
import { formatAbsolute } from '../../lib/dateFormat'

const TONE: Record<BuildQueue['state'], Tone> = {
  running: 'success',
  paused: 'warning',
  draining: 'info',
}

export function QueueStateChip({ queue }: { queue: BuildQueue }) {
  const { t } = useTranslation('deploys')
  const label = t('buildQueue.chip', {
    state: t(`buildQueue.state.${queue.state}`),
    used: queue.slots.used,
    total: queue.slots.total,
  })
  const parts: string[] = []
  if (queue.state !== 'running') {
    if (queue.actor) parts.push(t('buildQueue.by', { actor: queue.actor }))
    if (queue.since) parts.push(formatAbsolute(queue.since))
    if (queue.reason) parts.push(queue.reason)
  }
  return (
    <StatusPill
      size="sm"
      tone={TONE[queue.state]}
      label={label}
      live={queue.state === 'running' && queue.slots.used > 0}
      title={parts.length > 0 ? parts.join(', ') : undefined}
    />
  )
}
