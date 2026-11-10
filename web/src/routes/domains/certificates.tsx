import { createFileRoute } from '@tanstack/react-router'
import { useSuspenseQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { GlobeGlassIcon } from '@thesvg/ui-react'
import { Banner } from '@/components/ui/banner'
import { AutoHostList } from '../../components/certificates/AutoHostList'
import { CertBudgetCard } from '../../components/certificates/CertBudgetCard'
import { CertificateCenterTable } from '../../components/CertificateCenterTable'
import { CertificatesEmptyState } from '../../components/empty/CertificatesEmptyState'
import { HelpLink } from '../../components/HelpLink'
import { PageBanner } from '../../components/PageBanner'
import { LazyDomainsDiagram } from '../../components/banners/LazyDiagrams'
import { PageTemplate } from '../../components/shell/PageTemplate'
import { isInternalCert } from '../../lib/certStatus'
import { mergeCertificateCenterRows } from '../../lib/certificateCenter'
import { certificatesQueryOptions } from '../../queries/certificates'
import { domainsQueryOptions } from '../../queries/domains'

// The Certificates tab of the Domains area: same header and tab bar as the
// list, so moving between them never leaves the shell.
export const Route = createFileRoute('/domains/certificates')({
  loader: ({ context: { queryClient } }) =>
    Promise.all([
      queryClient.ensureQueryData(certificatesQueryOptions()),
      queryClient.ensureQueryData(domainsQueryOptions()),
    ]),
  component: CertificatesTab,
})

function DomainsHeader({ children }: { children: React.ReactNode }) {
  const { t } = useTranslation('domains')
  return (
    <PageTemplate
      title={t('page.title')}
      description={t('page.description')}
      banner={
        <PageBanner
          icon={<GlobeGlassIcon size="1.4em" />}
          titleKey="domains:page.title"
          descriptionKey="domains:page.description"
          action={
            <HelpLink path="/domains-and-ingress" label={t('page.helpLabel')} />
          }
          illustration={<LazyDomainsDiagram size="compact" />}
          size="compact"
        />
      }
    >
      {children}
    </PageTemplate>
  )
}

function CertificatesTab() {
  const { t } = useTranslation('settings')
  const { data: certificates } = useSuspenseQuery(certificatesQueryOptions())
  const { data: domains } = useSuspenseQuery(domainsQueryOptions())
  const rows = mergeCertificateCenterRows(domains, certificates)
  const expired = rows.filter((r) => r.cert?.status === 'expired').length
  const expiring = rows.filter(
    (r) => r.cert?.status === 'expiring_soon' && !isInternalCert(r.cert),
  ).length

  return (
    <DomainsHeader>
      {expired > 0 ? (
        <Banner
          variant="critical"
          title={t('certificatesPage.banner.expired', { count: expired })}
        >
          {t('certificatesPage.banner.expiredBody')}
        </Banner>
      ) : expiring > 0 ? (
        <Banner
          variant="warning"
          title={t('certificatesPage.banner.expiring', { count: expiring })}
        >
          {t('certificatesPage.banner.expiringBody')}
        </Banner>
      ) : null}
      <CertBudgetCard />
      {rows.length === 0 ? (
        <CertificatesEmptyState total={rows.length} />
      ) : (
        <CertificateCenterTable rows={rows} />
      )}
      <AutoHostList />
    </DomainsHeader>
  )
}
