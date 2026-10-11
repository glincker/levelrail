import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { DomainDnsCheck } from '../DomainDnsCheck'
import { certificatesQueryOptions } from '../../queries/certificates'
import type { DomainPolicies } from '../../queries/domainPolicyTypes'
import { PreviewPanel, Section } from './PolicyShared'

const SECTIONS = ['redirects', 'geo', 'headers', 'cache', 'forwarders'] as const

export function OverviewTab({
  app,
  domain,
  policies,
}: {
  app: string
  domain: string
  policies: DomainPolicies
}) {
  const { t } = useTranslation('domainPolicies')
  const { data: certs } = useQuery(certificatesQueryOptions())
  const cert = certs?.find((c) => c.domain === domain)

  return (
    <div className="grid gap-4 lg:grid-cols-2">
      <Section title={t('overview.statusTitle')}>
        <ul className="flex flex-wrap gap-2">
          {SECTIONS.map((s) => (
            <li key={s}>
              <Badge variant={policies.updated_at[s] ? 'success' : 'muted'}>
                {t(`tabs.${s}`)}:{' '}
                {policies.updated_at[s]
                  ? t('overview.custom')
                  : t('overview.default')}
              </Badge>
            </li>
          ))}
        </ul>
        <p className="text-xs text-muted-foreground">
          {policies.tls_real ? t('overview.tlsReal') : t('overview.tlsNotReal')}
        </p>
        {(policies.warnings ?? []).map((w) => (
          <Alert key={w}>
            <AlertDescription>{w}</AlertDescription>
          </Alert>
        ))}
      </Section>
      <Section title={t('overview.certTitle')}>
        {cert ? (
          <div className="space-y-1 text-sm">
            <Badge variant={cert.status === 'healthy' ? 'success' : 'warning'}>
              {cert.status}
            </Badge>
            <p className="text-xs text-muted-foreground">
              {t('overview.certDetail', {
                issuer: cert.issuer ?? cert.source,
                until: new Date(cert.not_after).toLocaleDateString(),
              })}
            </p>
          </div>
        ) : (
          <p className="text-sm text-muted-foreground">
            {t('overview.noCert')}
          </p>
        )}
      </Section>
      <Section title={t('overview.dnsTitle')}>
        <DomainDnsCheck appName={app} domain={domain} />
      </Section>
      <PreviewPanel steps={policies.preview} loading={false} />
    </div>
  )
}
