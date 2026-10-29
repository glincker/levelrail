import { useTranslation } from 'react-i18next'
import { ExternalLinkGlassIcon, GlobeGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { useAppNetwork } from '../queries/appNetwork'
import { BetaBadge } from './BetaBadge'
import { DnsProviderPicker } from './DnsProviderPicker'
import { FREE_PROVIDER_HOST } from '../lib/freeDnsHost'
import { FallbackBetaNote } from './FallbackBetaNote'

/** AutomaticDomainRow shows the zero-config hostname Levelrail assigned, so the Domains page agrees with the Overview. */
export function AutomaticDomainRow({ appName }: { appName: string }) {
  const { t } = useTranslation('networkProxy')
  const { data: network } = useAppNetwork(appName)
  const url = network?.fallback_url
  if (!url) return null
  const host = url.replace(/^https?:\/\//, '').replace(/\/$/, '')

  return (
    <div className="space-y-1 rounded-md border border-border px-3 py-2">
      <div className="flex flex-wrap items-center gap-2 text-sm">
        <GlobeGlassIcon
          className="size-4.5 text-muted-foreground"
          aria-hidden="true"
        />
        <span className="font-mono text-foreground">{host}</span>
        <Badge variant="muted">{t('automaticDomain.badge')}</Badge>
        {FREE_PROVIDER_HOST.test(host) ? (
          <BetaBadge hint={t('automaticDomain.betaHint')} />
        ) : null}
        <a
          href={url}
          target="_blank"
          rel="noreferrer"
          className="ml-auto inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground"
        >
          {t('automaticDomain.open')}
          <ExternalLinkGlassIcon className="size-4" aria-hidden="true" />
        </a>
      </div>
      <p className="text-xs text-muted-foreground">
        {t('automaticDomain.hint')}
      </p>
      <FallbackBetaNote url={url} />
      <DnsProviderPicker app={appName} />
    </div>
  )
}
