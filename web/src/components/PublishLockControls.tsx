import { useTranslation } from 'react-i18next'
import { LockGlassIcon, LockOpenGlassIcon } from '@thesvg/ui-react'
import {
  Alert,
  AlertAction,
  AlertDescription,
  AlertTitle,
} from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { usePublishLock, usePublishLockActions } from '../queries/publishLock'
import { RelativeTime } from './kit'

// PublishLockToggle is the app header control: lock or unlock publishing.
export function PublishLockToggle({ appName }: { appName: string }) {
  const { t } = useTranslation('deploySafety')
  const { data } = usePublishLock(appName)
  const { lock, unlock } = usePublishLockActions(appName)
  if (!data) {
    return null
  }
  const pending = lock.isPending || unlock.isPending
  const onError = (error: Error) =>
    toast.add({
      title: t('publishLock.error'),
      description: error.message,
      type: 'error',
    })
  return (
    <Button
      variant="outline"
      size="sm"
      disabled={pending}
      onClick={() =>
        data.locked
          ? unlock.mutate(undefined, { onError })
          : lock.mutate('', { onError })
      }
    >
      {data.locked ? (
        <LockOpenGlassIcon className="size-5" data-icon="inline-start" />
      ) : (
        <LockGlassIcon className="size-5" data-icon="inline-start" />
      )}
      {data.locked ? t('publishLock.unlock') : t('publishLock.lock')}
    </Button>
  )
}

// PublishLockBanner explains why new builds are not going live.
export function PublishLockBanner({ appName }: { appName: string }) {
  const { t } = useTranslation('deploySafety')
  const { data } = usePublishLock(appName)
  const { unlock } = usePublishLockActions(appName)
  if (!data?.locked) {
    return null
  }
  const held = data.held_builds.length
  return (
    <Alert>
      <LockGlassIcon size="1.4em" />
      <AlertTitle>{t('publishLock.bannerTitle')}</AlertTitle>
      <AlertDescription>
        {t('publishLock.bannerBody', {
          by: data.locked_by || '-',
          count: held,
        })}
        {data.locked_at ? (
          <>
            {' '}
            <RelativeTime at={data.locked_at} />
          </>
        ) : null}
        {data.reason
          ? ` ${t('publishLock.reason', { reason: data.reason })}`
          : ''}
      </AlertDescription>
      <AlertAction>
        <Button
          size="sm"
          variant="outline"
          disabled={unlock.isPending}
          onClick={() => unlock.mutate()}
        >
          {t('publishLock.unlock')}
        </Button>
      </AlertAction>
    </Alert>
  )
}
