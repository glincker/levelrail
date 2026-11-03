import { useTranslation } from 'react-i18next'
import { ExternalLinkGlassIcon } from '@thesvg/ui-react'
import {
  PROVIDER_DOCS,
  isKnownProvider,
  providerLabel,
  recordHost,
} from '../../lib/nameserverProviders'
import type { DomainCheckResult } from '../../queries/domainCheck'

// Plain steps for adding the A record in the dashboard of the provider that
// runs the zone's name servers. Renders nothing for an unknown provider.
export function ProviderRecordGuide({
  check,
  domain,
}: {
  check?: DomainCheckResult
  domain: string
}) {
  const { t } = useTranslation('dns')
  const id = check?.nameserver_provider
  if (!isKnownProvider(id)) return null
  const provider = providerLabel(id) ?? id
  const value = check?.expected_ipv4?.[0] ?? check?.expected_host ?? ''
  const docs = PROVIDER_DOCS[id]
  return (
    <section
      aria-label={t('providerGuide.title', { provider })}
      className="space-y-1.5 text-xs"
    >
      <h4 className="font-medium text-foreground">
        {t('providerGuide.title', { provider })}
      </h4>
      <ol className="list-decimal space-y-1 pl-5 text-muted-foreground">
        <li>{t(`providerGuide.where.${id}`, { provider })}</li>
        <li>
          {t('providerGuide.record', {
            host: recordHost(domain, check?.zone),
            value,
          })}
        </li>
        {id === 'cloudflare' ? <li>{t('providerGuide.proxyOff')}</li> : null}
      </ol>
      {docs ? (
        <a
          href={docs}
          target="_blank"
          rel="noreferrer noopener"
          className="inline-flex items-center gap-1 text-foreground underline-offset-2 hover:underline"
        >
          {t('providerGuide.docs', { provider })}
          <ExternalLinkGlassIcon className="size-4" aria-hidden="true" />
        </a>
      ) : null}
    </section>
  )
}
