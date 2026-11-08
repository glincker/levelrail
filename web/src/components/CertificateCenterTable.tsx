import { useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { ShieldCheckGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { toast } from '@/components/ui/toast'
import {
  certExpiryDate,
  certRenewalBadge,
  certStatusMeta,
  isInternalCert,
  isProxyManagedCert,
} from '../lib/certStatus'
import {
  filterCertRowsExpiringWithin,
  renewableCertRows,
  sortCertRowsByExpiry,
  type CertificateCenterRow,
} from '../lib/certificateCenter'
import { certificatesKeys } from '../queries/certificates'
import { renewDomainCertificate } from '../queries/domainTlsCert'
import { DomainTLSCertControl } from './DomainTLSCertControl'
import { RenewCertificateButton } from './RenewCertificateButton'

// One line per domain: name, one status chip, expiry, Renew now. Uploading
// a certificate of your own is a collapsed action so it stays out of the way.
function CertificateRow({
  row,
  selected,
  onToggle,
}: {
  row: CertificateCenterRow
  selected: boolean
  onToggle: () => void
}) {
  const { t } = useTranslation('https')
  const { t: ts } = useTranslation('settings')
  const { t: td } = useTranslation('domains')
  const [ownOpen, setOwnOpen] = useState(false)
  const { domain, appName, cert } = row
  const internal = cert ? isInternalCert(cert) : false
  const proxyManaged = cert
    ? isProxyManagedCert(cert)
    : row.tlsOwner === 'proxy'
  const renewalBadge = cert ? certRenewalBadge(cert) : null
  const canOwn = Boolean(appName) && !proxyManaged

  let chip = (
    <Badge variant="muted">{ts('certificatesPage.table.notIssued')}</Badge>
  )
  if (renewalBadge) {
    chip = (
      <Badge variant="destructive" title={renewalBadge.hint}>
        {renewalBadge.label}
      </Badge>
    )
  } else if (cert && internal && cert.status !== 'expired') {
    chip = (
      <Badge variant="muted" title={t('certTable.internalHint')}>
        {t('certTable.internalBadge')}
      </Badge>
    )
  } else if (cert) {
    chip = (
      <Badge variant={certStatusMeta[cert.status].variant}>
        {certStatusMeta[cert.status].label}
      </Badge>
    )
  } else if (proxyManaged) {
    chip = <Badge variant="info">{t('certTable.proxyBadge')}</Badge>
  }

  const nameClass = 'truncate font-mono text-sm font-medium text-foreground'
  return (
    <div className="py-2.5">
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
        <div className="flex min-w-0 flex-1 basis-56 items-center gap-2">
          <input
            type="checkbox"
            checked={selected}
            onChange={onToggle}
            disabled={renewableCertRows([row]).length === 0}
            aria-label={ts('certificatesPage.bulk.select', { domain })}
          />
          {appName ? (
            <Link
              to="/apps/$name/domains/$domain"
              params={{ name: appName, domain }}
              className={`${nameClass} hover:underline`}
              title={td('detail.openDomain', { domain })}
            >
              {domain}
            </Link>
          ) : (
            <span className={nameClass}>{domain}</span>
          )}
          {appName ? null : (
            <Badge variant="muted">{ts('certificatesPage.table.noApp')}</Badge>
          )}
        </div>
        {chip}
        <span
          className="w-28 text-xs text-muted-foreground"
          title={cert?.not_after}
        >
          {cert && !internal ? certExpiryDate(cert.not_after) : '-'}
        </span>
        <div className="ml-auto flex flex-wrap items-center gap-2">
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
          {canOwn ? (
            <Button
              type="button"
              size="xs"
              variant="ghost"
              aria-expanded={ownOpen}
              onClick={() => {
                setOwnOpen((o) => !o)
              }}
            >
              {t('certTable.useOwn')}
            </Button>
          ) : null}
        </div>
      </div>
      {canOwn && appName && ownOpen ? (
        <div className="pt-2">
          <DomainTLSCertControl appName={appName} domain={domain} />
        </div>
      ) : null}
    </div>
  )
}

const DEFAULT_WINDOW_DAYS = Number(
  import.meta.env.VITE_CERT_EXPIRY_FILTER_DAYS ?? 14,
)

export function CertificateCenterTable({
  rows,
}: {
  rows: CertificateCenterRow[]
}) {
  const { t } = useTranslation('settings')
  const queryClient = useQueryClient()
  const [windowOn, setWindowOn] = useState(false)
  const [days, setDays] = useState(DEFAULT_WINDOW_DAYS)
  const [byExpiry, setByExpiry] = useState(false)
  const [picked, setPicked] = useState<Set<string>>(new Set())
  const [busy, setBusy] = useState(false)

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

  let shown = windowOn ? filterCertRowsExpiringWithin(rows, days) : rows
  if (byExpiry) shown = sortCertRowsByExpiry(shown)
  const renewable = renewableCertRows(shown)
  const targets = renewable.filter((r) => picked.has(r.domain))

  const toggle = (domain: string) =>
    setPicked((prev) => {
      const next = new Set(prev)
      if (next.has(domain)) next.delete(domain)
      else next.add(domain)
      return next
    })

  const renewSelected = async () => {
    setBusy(true)
    const results = await Promise.allSettled(
      targets.map((r) => renewDomainCertificate(r.appName ?? '', r.domain)),
    )
    const failed = results.filter((r) => r.status === 'rejected').length
    void queryClient.invalidateQueries({ queryKey: certificatesKeys.all })
    toast.add({
      title: t('certificatesPage.bulk.done', {
        count: results.length - failed,
      }),
      description:
        failed > 0 ? t('certificatesPage.bulk.failed', { count: failed }) : '',
      type: failed > 0 ? 'error' : 'success',
    })
    setPicked(new Set())
    setBusy(false)
  }

  return (
    <div className="space-y-2">
      <div className="flex flex-wrap items-center gap-3 text-sm">
        <label className="flex items-center gap-2">
          <input
            type="checkbox"
            checked={windowOn}
            onChange={(e) => setWindowOn(e.target.checked)}
          />
          {t('certificatesPage.bulk.expiresIn')}
        </label>
        <input
          type="number"
          min={1}
          value={days}
          disabled={!windowOn}
          aria-label={t('certificatesPage.bulk.days')}
          onChange={(e) => setDays(Math.max(1, Number(e.target.value) || 1))}
          className="w-16 rounded-md border bg-background px-2 py-1"
        />
        <span className="text-muted-foreground">
          {t('certificatesPage.bulk.daysUnit')}
        </span>
        <label className="flex items-center gap-2">
          <input
            type="checkbox"
            checked={byExpiry}
            onChange={(e) => setByExpiry(e.target.checked)}
          />
          {t('certificatesPage.bulk.sortExpiry')}
        </label>
        <Button
          type="button"
          size="sm"
          className="ml-auto"
          disabled={busy || targets.length === 0}
          onClick={() => void renewSelected()}
        >
          {t('certificatesPage.bulk.renew', { count: targets.length })}
        </Button>
      </div>
      <div className="divide-y divide-border rounded-lg border border-border bg-card px-4">
        {shown.map((row) => (
          <CertificateRow
            key={row.domain}
            row={row}
            selected={picked.has(row.domain)}
            onToggle={() => toggle(row.domain)}
          />
        ))}
      </div>
    </div>
  )
}
