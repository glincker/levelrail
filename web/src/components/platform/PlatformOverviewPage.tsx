import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  CloudCheckGlassIcon,
  GlobeGlassIcon,
  LockGlassIcon,
  PlugGlassIcon,
} from '@thesvg/ui-react'
import { buttonVariants } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { PlatformSettingsSearch } from './PlatformSettingsSearch'
import {
  PlatformStatusTile,
  type PlatformStatusTileProps,
} from './PlatformStatusTile'
import { usePlatformStatus } from './usePlatformStatus'

export function PlatformOverviewPage() {
  const { t } = useTranslation('platformSettings')
  const s = usePlatformStatus({ connectivity: true })

  const https: PlatformStatusTileProps = {
    id: 'https',
    icon: <LockGlassIcon size="1.4em" />,
    title: t('overview.https.title'),
  }
  if (s.httpsOff !== undefined) {
    https.chip = s.httpsOff
      ? { label: t('overview.https.off'), tone: 'warning' }
      : { label: t('overview.https.on'), tone: 'success' }
    https.line = s.httpsOff
      ? t('overview.https.offLine')
      : t('overview.https.onLine', { domain: s.domain })
    if (s.httpsOff) {
      https.action = (
        <Link
          to="/settings/platform/https"
          className={cn(buttonVariants({ size: 'sm' }))}
        >
          {t('overview.https.enable')}
        </Link>
      )
    }
  }

  const ingress: PlatformStatusTileProps = {
    id: 'ingress',
    icon: <PlugGlassIcon size="1.4em" />,
    title: t('overview.ingress.title'),
  }
  if (s.connectivity) {
    const ok = s.connectivity.guidance === 'ok'
    ingress.chip = ok
      ? { label: t('overview.ingress.healthy'), tone: 'success' }
      : { label: t('overview.ingress.attention'), tone: 'warning' }
    ingress.line = ok
      ? s.connectivity.host
        ? t('overview.ingress.healthyLine', { host: s.connectivity.host })
        : t('overview.ingress.healthyNoHost')
      : t('overview.ingress.attentionLine')
  }

  const addresses: PlatformStatusTileProps = {
    id: 'addresses',
    icon: <GlobeGlassIcon size="1.4em" />,
    title: t('overview.addresses.title'),
  }
  if (s.dashboardLoaded) {
    addresses.chip = s.dashboardUrl
      ? { label: t('overview.addresses.set'), tone: 'success' }
      : { label: t('overview.addresses.unset'), tone: 'warning' }
    addresses.line = s.dashboardUrl
      ? t('overview.addresses.setLine', { url: s.dashboardUrl })
      : t('overview.addresses.unsetLine')
  }

  const dns: PlatformStatusTileProps = {
    id: 'dns-providers',
    icon: <CloudCheckGlassIcon size="1.4em" />,
    title: t('overview.dns.title'),
  }
  if (s.dnsLoaded) {
    const connected = s.providers.length > 0
    dns.chip = connected
      ? {
          label: t('overview.dns.connected', {
            providers: s.providers.join(', '),
          }),
          tone: 'success',
        }
      : { label: t('overview.dns.none'), tone: 'muted' }
    dns.line = connected
      ? t('overview.dns.connectedLine')
      : t('overview.dns.noneLine')
  }

  return (
    <>
      <ul
        aria-label={t('overview.tilesLabel')}
        className="grid grid-cols-1 gap-3 p-0 sm:grid-cols-2"
      >
        <PlatformStatusTile {...https} />
        <PlatformStatusTile {...ingress} />
        <PlatformStatusTile {...addresses} />
        <PlatformStatusTile {...dns} />
      </ul>
      <PlatformSettingsSearch />
    </>
  )
}
