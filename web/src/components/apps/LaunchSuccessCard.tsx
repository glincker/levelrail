import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { XGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { AppQrPanel } from './AppQrPanel'
import { useUrlReachability } from '../overview/useUrlReachability'
import { useAppUrl } from '../overview/useAppUrl'
import type { AppDetail } from '../../types/appDetail'
import type { DeployAttempt } from '../../types/deployAttempt'

const storageKey = (app: string) => `launch-success-dismissed:${app}`

function readDismissed(app: string): boolean {
  try {
    return localStorage.getItem(storageKey(app)) === '1'
  } catch {
    return false
  }
}

// Shown once after the first successful deploy: the live URL, copy, a QR
// code to open it on a phone, and the link to share.
export function LaunchSuccessCard({
  app,
  attempts,
}: {
  app: AppDetail
  attempts: DeployAttempt[]
}) {
  const { t } = useTranslation('appSetup')
  const url = useAppUrl(app)
  const [dismissed, setDismissed] = useState(() => readDismissed(app.name))
  const succeeded = attempts.filter((a) => a.status === 'succeeded').length
  const { reach, shareUrl } = useUrlReachability(app, url)
  if (dismissed || succeeded !== 1 || attempts[0]?.status !== 'succeeded') {
    return null
  }
  // Only a ready app earns the success banner; a down or pending one stays quiet.
  if (!url || (reach.state !== 'reachable' && reach.state !== 'local_only')) {
    return null
  }
  return (
    <section
      aria-label={t('launch.title')}
      className="flex items-start gap-3 rounded-xl border border-border bg-card px-4 py-3"
    >
      <div className="min-w-0 flex-1 space-y-2">
        <p className="text-sm font-medium">{t('launch.title')}</p>
        <AppQrPanel
          appName={app.name}
          url={url}
          shareUrl={shareUrl}
          reach={reach}
        />
      </div>
      <Button
        type="button"
        variant="ghost"
        size="icon-sm"
        aria-label={t('launch.dismiss')}
        onClick={() => {
          setDismissed(true)
          try {
            localStorage.setItem(storageKey(app.name), '1')
          } catch {
            // Dismissal only lasts this visit when storage is blocked.
          }
        }}
      >
        <XGlassIcon className="size-4.5" aria-hidden="true" />
      </Button>
    </section>
  )
}
