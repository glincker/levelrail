import { ShieldCheckIcon } from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { EmptyState } from '@/components/ui/empty-state'
import {
  CERT_RENEWAL_STALLED_HINT,
  certExpiryLabel,
  certRenewalBadge,
  certStatusMeta,
} from '../lib/certStatus'
import type { CertificateCenterRow } from '../lib/certificateCenter'
import { DomainTLSCertControl } from './DomainTLSCertControl'
import { RenewCertificateButton } from './RenewCertificateButton'

function CertificateRow({ row }: { row: CertificateCenterRow }) {
  const { domain, appName, cert } = row
  const renewalBadge = cert ? certRenewalBadge(cert) : null

  return (
    <div className="space-y-2 py-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <p className="truncate font-medium text-foreground">{domain}</p>
            {appName ? (
              <Badge variant="outline">{appName}</Badge>
            ) : (
              <Badge variant="muted">No app</Badge>
            )}
          </div>
          <p className="text-xs text-muted-foreground">
            {cert
              ? certExpiryLabel(cert.not_after)
              : appName
                ? 'Not yet issued'
                : 'Certificate remains from a domain no longer configured on any app'}
          </p>
        </div>
        <div className="flex items-center gap-2">
          {cert ? (
            <Badge variant={cert.source === 'custom' ? 'outline' : 'muted'}>
              {cert.source === 'custom' ? 'Custom' : 'ACME / internal'}
            </Badge>
          ) : null}
          {cert ? (
            <Badge variant={certStatusMeta[cert.status].variant}>
              {certStatusMeta[cert.status].label}
            </Badge>
          ) : null}
          {renewalBadge ? (
            <Badge variant="destructive" title={CERT_RENEWAL_STALLED_HINT}>
              {renewalBadge.label}
            </Badge>
          ) : null}
          {appName && (!cert || cert.source === 'acme') ? (
            <RenewCertificateButton appName={appName} domain={domain} />
          ) : null}
        </div>
      </div>
      {appName ? (
        <DomainTLSCertControl appName={appName} domain={domain} />
      ) : null}
    </div>
  )
}

export function CertificateCenterTable({
  rows,
}: {
  rows: CertificateCenterRow[]
}) {
  if (rows.length === 0) {
    return (
      <EmptyState
        className="py-12"
        icon={<ShieldCheckIcon className="size-5" />}
        title="No domains configured yet"
        description="Add a domain to an app to see its certificate here, automatic or custom."
      />
    )
  }

  return (
    <div className="divide-y divide-border rounded-lg border border-border px-4">
      {rows.map((row) => (
        <CertificateRow key={row.domain} row={row} />
      ))}
    </div>
  )
}
