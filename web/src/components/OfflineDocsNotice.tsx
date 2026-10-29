import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { XGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { useBrand } from '@/hooks/useBrand'
import { routePathToDocsSiteUrl, routePathWithHash } from '@/lib/docsPathMapper'

interface OfflineDocsNoticeProps {
  routePath: string
}

const DISMISS_KEY = 'levelrail:offline-notice-dismissed'

export function OfflineDocsNotice({ routePath }: OfflineDocsNoticeProps) {
  const { t } = useTranslation('help')
  const brand = useBrand()
  const [dismissed, setDismissed] = useState(() => {
    try {
      return sessionStorage.getItem(DISMISS_KEY) === 'true'
    } catch {
      return false
    }
  })

  if (!brand.DocsURL || dismissed) return null

  const handleDismiss = () => {
    try {
      sessionStorage.setItem(DISMISS_KEY, 'true')
    } catch {
      // Ignore if sessionStorage is unavailable
    }
    setDismissed(true)
  }

  const currentHash = typeof window !== 'undefined' ? window.location.hash : ''
  const docsHref = routePathToDocsSiteUrl(
    routePathWithHash(routePath, currentHash.slice(1)),
    brand.DocsURL,
  )

  return (
    <div className="mb-4 flex items-start gap-3 rounded-lg border border-border bg-muted/50 p-3 text-sm">
      <div className="flex-1 text-muted-foreground">
        <p className="font-medium text-foreground mb-1">
          {t('offlineNotice.title')}
        </p>
        <p className="mb-2">
          {t('offlineNotice.description')}
          {docsHref && (
            <>
              {' '}
              <a
                href={docsHref}
                target="_blank"
                rel="noopener noreferrer"
                className="underline hover:text-foreground text-muted-foreground"
              >
                {brand.DocsURL}
              </a>
            </>
          )}
        </p>
      </div>
      <Button
        type="button"
        variant="ghost"
        size="icon-sm"
        onClick={handleDismiss}
        title={t('offlineNotice.dismiss')}
        className="shrink-0"
      >
        <XGlassIcon className="size-5" />
      </Button>
    </div>
  )
}
