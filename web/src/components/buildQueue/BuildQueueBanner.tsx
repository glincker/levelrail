import { useTranslation } from 'react-i18next'
import { PlayGlassIcon } from '@thesvg/ui-react'
import { Banner } from '@/components/ui/banner'
import { Button } from '@/components/ui/button'
import { RelativeTime } from '@/components/kit'
import { useAbilities } from '@/hooks/useAbilities'
import { useBuildQueue } from '../../queries/buildQueue'
import { useQueueChange } from './useQueueToasts'

// Shown only while the queue is not running; one level, no nested cards.
export function BuildQueueBanner() {
  const { t } = useTranslation('deploys')
  const { data: queue } = useBuildQueue()
  const { can } = useAbilities()
  const change = useQueueChange()
  if (!queue || queue.state === 'running') return null

  const canResume = can('write:sensitive')
  return (
    <Banner
      variant="warning"
      title={t(`buildQueue.banner.${queue.state}`)}
      action={
        canResume ? (
          <Button
            type="button"
            size="sm"
            variant="outline"
            writeAction
            disabled={change.pending}
            onClick={() => {
              change.run('resume')
            }}
          >
            <PlayGlassIcon size="1.4em" aria-hidden="true" />
            {t('buildQueue.resume')}
          </Button>
        ) : undefined
      }
    >
      <p>
        {queue.actor ? `${t('buildQueue.by', { actor: queue.actor })} ` : ''}
        {queue.since ? <RelativeTime at={queue.since} /> : null}
        {queue.reason ? `: ${queue.reason}` : ''}
      </p>
      {queue.auto_resume_at ? (
        <p>
          {t('buildQueue.autoResume')}{' '}
          <RelativeTime at={queue.auto_resume_at} />
        </p>
      ) : null}
    </Banner>
  )
}
