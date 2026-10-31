import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { buttonVariants } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { usePlatformStatus } from './usePlatformStatus'

// Slim HTTPS and dashboard address state for the top of Domains, linking
// into the platform pages. No accordion: the settings live on their own pages.
export function PlatformStatusStrip() {
  const { t } = useTranslation('platformSettings')
  const { httpsOff, dashboardUrl, dashboardLoaded } = usePlatformStatus()
  return (
    <div
      role="region"
      aria-label={t('strip.label')}
      className="flex flex-wrap items-center gap-x-4 gap-y-2 rounded-lg border border-border bg-card px-4 py-2 text-sm"
    >
      {httpsOff === undefined ? null : httpsOff ? (
        <span className="flex items-center gap-2">
          <Badge variant="warning">{t('strip.httpsOff')}</Badge>
          <Link
            to="/settings/platform/https"
            className={cn(buttonVariants({ size: 'xs', variant: 'outline' }))}
          >
            {t('strip.enable')}
          </Link>
        </span>
      ) : (
        <Link to="/settings/platform/https">
          <Badge variant="success">{t('strip.httpsOn')}</Badge>
        </Link>
      )}
      <span className="flex min-w-0 items-center gap-2 text-muted-foreground">
        {t('strip.address')}
        {dashboardLoaded ? (
          <Link
            to="/settings/platform/addresses"
            hash="dashboard-url"
            className="truncate font-mono text-foreground hover:underline"
          >
            {dashboardUrl || t('strip.addressUnset')}
          </Link>
        ) : null}
      </span>
      <Link
        to="/settings/platform"
        className="ml-auto text-muted-foreground hover:text-foreground hover:underline"
      >
        {t('strip.manage')}
      </Link>
    </div>
  )
}
