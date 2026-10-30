import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { ShieldCheckGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { EmptyState } from '@/components/ui/empty-state'
import {
  certExpiryLabel,
  certRenewalBadge,
  certStatusMeta,
  isInternalCert,
  isProxyManagedCert,
} from '../lib/certStatus'
import type { CertificateCenterRow } from '../lib/certificateCenter'
import { DomainTLSCertControl } from './DomainTLSCertControl'
import { RenewCertificateButton } from './RenewCertificateButton'

function CertificateRow({ row }: { row: CertificateCenterRow }) {
  const { t } = useTranslation('https')
  const { t: ts } = useTranslation('settings')
  const { domain, appName, cert } = row
  const internal = cert ? isInternalCert(cert) : false
  const proxyManaged = cert
    ? isProxyManagedCert(cert)
    : row.tlsOwner === 'proxy'
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
              <Badge variant="muted">
                {ts('certificatesPage.table.noApp')}
              </Badge>
            )}
          </div>
          <p className="text-xs text-muted-foreground">
            {cert
              ? internal
                ? t('certTable.internalRenews')
                : proxyManaged
                  ? `${certExpiryLabel(cert.not_after)}. ${t('certTable.proxyRenews')}`
                  : certExpiryLabel(cert.not_after)
              : proxyManaged
                ? t('certTable.proxyUnknown')
                : appName
                  ? ts('certificatesPage.table.notIssued')
                  : ts('certificatesPage.table.orphaned')}
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
                : proxyManaged
                  ? t('certTable.proxyBadge')
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
          {appName && !proxyManaged && (!cert || cert.source === 'acme') ? (
            <RenewCertificateButton appName={appName} domain={domain} />
          ) : null}
          {proxyManaged ? (
            <Link to="/network/proxy" className="text-xs underline">
              {t('certTable.proxyLink')}
            </Link>
          ) : null}
        </div>
      </div>
      {appName && !proxyManaged ? (
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
  const { t } = useTranslation('settings')
  if (rows.length === 0) {
    return (
      <EmptyState
        className="gap-2 border-0 bg-transparent py-4"
        icon={<ShieldCheckGlassIcon className="size-6 text-tone-success" />}
        title={t('certificatesPage.empty.title')}
        description={t('certificatesPage.empty.body')}
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
