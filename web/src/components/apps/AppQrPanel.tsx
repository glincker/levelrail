import { Suspense, lazy, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import type { UrlReachability } from '../../lib/urlReachability'

// Lazy so the QR encoder stays out of the main bundle.
const BrandQr = lazy(() => import('../security/BrandQr'))

/** QR plus copy and share for one address, honest about whether a phone can open it. */
export function AppQrPanel({
  appName,
  url,
  shareUrl,
  reach,
  size = 112,
}: {
  appName: string
  url: string
  shareUrl: string | null
  reach: UrlReachability
  size?: number
}) {
  const { t } = useTranslation('appSetup')
  const { t: tStatus } = useTranslation('appOverview')
  const [copied, setCopied] = useState(false)
  const target = shareUrl ?? url
  return (
    <div className="flex flex-wrap items-center gap-4">
      {shareUrl ? (
        <Suspense
          fallback={
            <Skeleton
              className="size-28 rounded-lg"
              role="img"
              aria-label={t('launch.qrLoading')}
            />
          }
        >
          <div className="shrink-0 rounded-lg" data-testid="app-qr">
            <BrandQr value={shareUrl} size={size} label={t('launch.qrLabel')} />
          </div>
        </Suspense>
      ) : null}
      <div className="min-w-0 flex-1 space-y-1">
        <p className="text-muted-foreground text-xs">
          {shareUrl
            ? reach.state === 'reachable'
              ? t('launch.body')
              : tStatus('status.qr.fallback')
            : tStatus(
                `status.reason.${reach.state === 'reachable' ? 'unknown' : reach.state}`,
              )}
        </p>
        {reach.state === 'reachable' || shareUrl ? (
          <a
            href={target}
            target="_blank"
            rel="noreferrer"
            className="block truncate text-sm underline"
          >
            {target}
          </a>
        ) : (
          <span className="block truncate text-sm text-muted-foreground">
            {target}
          </span>
        )}
        <div className="flex flex-wrap gap-2 pt-1">
          <Button
            type="button"
            size="sm"
            onClick={() =>
              void navigator.clipboard.writeText(target).then(() => {
                setCopied(true)
              })
            }
          >
            {copied ? t('launch.copied') : t('launch.copy')}
          </Button>
          {shareUrl ? (
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() =>
                void navigator.clipboard.writeText(
                  `${t('launch.shareText', { name: appName })} ${shareUrl}`,
                )
              }
            >
              {t('launch.share')}
            </Button>
          ) : null}
        </div>
      </div>
    </div>
  )
}
