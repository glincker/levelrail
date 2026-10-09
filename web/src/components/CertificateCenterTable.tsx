import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { ShieldCheckIcon } from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { EmptyState } from '@/components/ui/empty-state'
import {
  certExpiryLabel,
  certRenewalBadge,
  certStatusMeta,
  isInternalCert,
} from '../lib/certStatus'
import type { CertificateCenterRow } from '../lib/certificateCenter'
import { DomainTLSCertControl } from './DomainTLSCertControl'
import { RenewCertificateButton } from './RenewCertificateButton'

function CertificateRow({ row }: { row: CertificateCenterRow }) {
  const { t } = useTranslation('https')
  const { domain, appName, cert } = row
  const internal = cert ? isInternalCert(cert) : false
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
              ? internal
                ? t('certTable.internalRenews')
                : certExpiryLabel(cert.not_after)
              : appName
                ? 'Not yet issued'
                : 'Certificate remains from a domain no longer configured on any app'}
          </p>
        </div>
        <div className="flex items-center gap-2">
          {cert ? (
            <Badge
              variant={cert.source === 'custom' ? 'outline' : 'muted'}
              title={internal ? t('certTable.internalHint') : undefined}
            >
              {cert.source === 'custom'
                ? t('certTable.custom')
                : internal
                  ? t('certTable.internalBadge')
                  : t('certTable.acmeBadge')}
            </Badge>
          ) : null}
          {cert && !(internal && cert.status !== 'expired') ? (
            <Badge variant={certStatusMeta[cert.status].variant}>
              {certStatusMeta[cert.status].label}
            </Badge>
          ) : null}
          {renewalBadge ? (
            <Badge variant="destructive" title={renewalBadge.hint}>
              {renewalBadge.label}
            </Badge>
          ) : null}
          {internal ? (
            <Link to="/domains" className="text-xs underline">
              {t('certTable.getTrusted')}
            </Link>
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
