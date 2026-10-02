import { createFileRoute } from '@tanstack/react-router'
import { useSuspenseQuery } from '@tanstack/react-query'
import { ShieldCheckIcon } from '@phosphor-icons/react/dist/ssr'
import { certificatesQueryOptions } from '../../queries/certificates'
import { domainsQueryOptions } from '../../queries/domains'
import { mergeCertificateCenterRows } from '../../lib/certificateCenter'
import { CertificateCenterTable } from '../../components/CertificateCenterTable'
import { TableSkeleton } from '@/components/ui/table-skeleton'
import { PageHeader } from '@/components/shell/PageHeader'

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
  const { data: certificates } = useSuspenseQuery(certificatesQueryOptions())
  const { data: domains } = useSuspenseQuery(domainsQueryOptions())
  const rows = mergeCertificateCenterRows(domains, certificates)

  return (
    <div className="space-y-6">
      <div className="flex items-start gap-3">
        <div className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
          <ShieldCheckIcon className="size-4" aria-hidden="true" />
        </div>
        <div className="min-w-0 flex-1">
          <PageHeader
            title="TLS certificates"
            description="Every domain's certificate, automatic or custom, with expiry and a renew action. Automatic certificates are issued and renewed by the embedded Caddy ingress; use Renew now to force an immediate re-issuance instead of waiting for the next scheduled check."
          />
        </div>
      </div>
      <CertificateCenterTable rows={rows} />
    </div>
  )
}

function CertificatesPending() {
  return (
    <div className="space-y-6">
      <h1 className="text-lg font-semibold text-foreground">
        TLS certificates
      </h1>
      <TableSkeleton columnCount={4} />
    </div>
  )
}
