import { Suspense, lazy, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { XGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { useAppNetwork } from '../../queries/appNetwork'
import type { AppDetail } from '../../types/appDetail'
import type { DeployAttempt } from '../../types/deployAttempt'

// Lazy so the QR encoder stays out of the main bundle.
const QrCodeSvg = lazy(() => import('../security/QrCodeSvg'))

const QR_SIZE = 112
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
  const { data: network } = useAppNetwork(app.name)
  const [dismissed, setDismissed] = useState(() => readDismissed(app.name))
  const [copied, setCopied] = useState(false)
  const succeeded = attempts.filter((a) => a.status === 'succeeded').length
  const [domain] = app.domains ?? []
  const url = domain ? `https://${domain}` : network?.fallback_url
  if (dismissed || succeeded !== 1 || attempts[0]?.status !== 'succeeded') {
    return null
  }
  if (!url) return null
  return (
    <section
      aria-label={t('launch.title')}
      className="flex flex-wrap items-center gap-4 rounded-xl border border-border bg-card px-4 py-3"
    >
      <div className="rounded-lg bg-white p-2">
        <Suspense
          fallback={
            <Skeleton
              className="size-[112px]"
              role="img"
              aria-label={t('launch.qrLoading')}
            />
          }
        >
          <QrCodeSvg value={url} size={QR_SIZE} label={t('launch.qrLabel')} />
        </Suspense>
      </div>
      <div className="min-w-0 flex-1 space-y-1">
        <p className="text-sm font-medium">{t('launch.title')}</p>
        <p className="text-muted-foreground text-xs">{t('launch.body')}</p>
        <a
          href={url}
          target="_blank"
          rel="noreferrer"
          className="block truncate text-sm underline"
        >
          {url}
        </a>
        <div className="flex flex-wrap gap-2 pt-1">
          <Button
            type="button"
            size="sm"
            onClick={() =>
              void navigator.clipboard.writeText(url).then(() => {
                setCopied(true)
              })
            }
          >
            {copied ? t('launch.copied') : t('launch.copy')}
          </Button>
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() =>
              void navigator.clipboard.writeText(
                `${t('launch.shareText', { name: app.name })} ${url}`,
              )
            }
          >
            {t('launch.share')}
          </Button>
        </div>
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
