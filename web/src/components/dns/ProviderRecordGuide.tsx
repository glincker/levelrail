import { useTranslation } from 'react-i18next'
import {
  CheckGlassIcon,
  CopyGlassIcon,
  ExternalLinkGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { useCopyToClipboard } from '../../hooks/useCopyToClipboard'
import {
  PROVIDER_DOCS,
  isKnownProvider,
  providerLabel,
  recordHost,
} from '../../lib/nameserverProviders'
import type { DomainCheckResult } from '../../queries/domainCheck'

function CopyField({ label, text }: { label: string; text: string }) {
  const { copied, copy } = useCopyToClipboard()
  const { t } = useTranslation('dns')
  return (
    <Button
      size="sm"
      variant="outline"
      aria-label={label}
      onClick={() => copy(text)}
    >
      {copied ? (
        <CheckGlassIcon size="1.4em" />
      ) : (
        <CopyGlassIcon size="1.4em" />
      )}
      {copied ? t('wizard.copied') : label}
    </Button>
  )
}

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
  const host = recordHost(domain, check?.zone)
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
            host,
            value,
          })}
        </li>
        {id === 'cloudflare' ? <li>{t('providerGuide.proxyOff')}</li> : null}
      </ol>
      {value ? (
        <div className="flex flex-wrap gap-2">
          <CopyField label={t('providerGuide.copyHost')} text={host} />
          <CopyField label={t('providerGuide.copyValue')} text={value} />
        </div>
      ) : null}
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
