import { createFileRoute } from '@tanstack/react-router'
import { useSuspenseQuery } from '@tanstack/react-query'
import { ShieldCheckGlassIcon } from '@thesvg/ui-react'
import { certificatesQueryOptions } from '../../queries/certificates'
import { domainsQueryOptions } from '../../queries/domains'
import { mergeCertificateCenterRows } from '../../lib/certificateCenter'
import { CertificateCenterTable } from '../../components/CertificateCenterTable'
import { TableSkeleton } from '@/components/ui/table-skeleton'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { Badge } from '@/components/ui/badge'
import { Banner } from '@/components/ui/banner'
import { useTranslation } from 'react-i18next'
import { isInternalCert } from '../../lib/certStatus'
import { SettingsPageTitle } from '../../components/settings/SettingsPageTitle'

// The certificate center: every domain's TLS certificate in one place,
// across every app, with expiry, issuer (ACME/internal vs an
// operator-uploaded one), and a "Renew now" action. settings/general.tsx's
// CertificatesCard stays as the passive summary widget; this route is
// where an operator actually acts on a certificate instead of only
// seeing its status.
export const Route = createFileRoute('/settings/certificates')({
  loader: ({ context: { queryClient } }) =>
    Promise.all([
      queryClient.ensureQueryData(certificatesQueryOptions()),
      queryClient.ensureQueryData(domainsQueryOptions()),
    ]),
  component: CertificatesPage,
  pendingComponent: CertificatesPending,
})

function CertificatesPage() {
  const { t } = useTranslation('settings')
  const { data: certificates } = useSuspenseQuery(certificatesQueryOptions())
  const { data: domains } = useSuspenseQuery(domainsQueryOptions())
  const rows = mergeCertificateCenterRows(domains, certificates)
  const expired = rows.filter((r) => r.cert?.status === 'expired').length
  const expiring = rows.filter(
    (r) => r.cert?.status === 'expiring_soon' && !isInternalCert(r.cert),
  ).length

  let banner = null
  if (expired > 0) {
    banner = (
      <Banner
        variant="critical"
        title={t('certificatesPage.banner.expired', { count: expired })}
      >
        {t('certificatesPage.banner.expiredBody')}
      </Banner>
    )
  } else if (expiring > 0) {
    banner = (
      <Banner
        variant="warning"
        title={t('certificatesPage.banner.expiring', { count: expiring })}
      >
        {t('certificatesPage.banner.expiringBody')}
      </Banner>
    )
  }

  return (
    <PageTemplate
      icon={<ShieldCheckGlassIcon className="text-tone-success" size="1.4em" />}
      eyebrow={t('certificatesPage.eyebrow')}
      title={t('certificatesPage.title')}
      description={t('certificatesPage.description')}
      helpPath="/domains-and-ingress"
      helpLabel={t('certificatesPage.help')}
      status={
        <Badge variant={expired + expiring > 0 ? 'warning' : 'success'}>
          {t('certificatesPage.count', { count: rows.length })}
        </Badge>
      }
      banners={banner}
    >
      <CertificateCenterTable rows={rows} />
    </PageTemplate>
  )
}

function CertificatesPending() {
  return (
    <div className="space-y-6">
      <SettingsPageTitle id="certificates" />
      <TableSkeleton columnCount={4} />
    </div>
  )
}
