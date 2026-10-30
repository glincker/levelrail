import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { DnsHostTile } from '@/components/dns/DnsHostChip'
import type { DnsZone } from '@/types/dns'
import { delegationChipKey, hostKey, type ProviderId } from './providers'

const CHIP: Record<string, 'success' | 'warning' | 'muted'> = {
  delegated: 'success',
  partial: 'warning',
  notDelegated: 'warning',
  unknown: 'muted',
}

export function DelegationChip({ zone }: { zone: DnsZone }) {
  const { t } = useTranslation('dnsProviders')
  const key = delegationChipKey(zone.delegated)
  const reason = zone.delegation_reason
    ? t(`delegation.reason.${zone.delegation_reason}`)
    : ''
  const label = t(`delegation.${key}`)
  return (
    <Badge
      variant={CHIP[key]}
      title={reason || undefined}
      aria-label={
        reason ? t('delegation.chipLabel', { state: label, reason }) : label
      }
    >
      {label}
    </Badge>
  )
}

function statusKey(
  s: string | undefined,
): 'active' | 'pending' | 'moved' | 'unknown' {
  const v = (s ?? '').toLowerCase()
  return v === 'active' || v === 'pending' || v === 'moved' ? v : 'unknown'
}

const GRID =
  'grid grid-cols-[minmax(0,2fr)_minmax(0,1.4fr)_auto] gap-3 sm:grid-cols-[minmax(0,2fr)_minmax(0,1.4fr)_5rem_6rem_4rem_7rem]'

export function ZonesTable({
  zones,
  onOpen,
}: {
  zones: DnsZone[]
  onOpen: (z: DnsZone) => void
}) {
  const { t } = useTranslation('dnsProviders')
  return (
    <div
      role="table"
      aria-label={t('zones.heading')}
      className="rounded-lg border border-border text-sm"
    >
      <div
        role="row"
        className={`${GRID} hidden border-b border-border px-3 py-2 text-xs font-medium text-muted-foreground sm:grid`}
      >
        <span role="columnheader">{t('zones.name')}</span>
        <span role="columnheader">{t('zones.provider')}</span>
        <span role="columnheader">{t('zones.type')}</span>
        <span role="columnheader">{t('zones.status')}</span>
        <span role="columnheader">{t('zones.records')}</span>
        <span role="columnheader">{t('zones.delegation')}</span>
      </div>
      {zones.map((z) => {
        const s = statusKey(z.status)
        return (
          <div
            role="row"
            key={`${z.provider}:${z.id}`}
            className={`${GRID} items-center border-b border-border px-3 py-2 last:border-b-0 hover:bg-muted`}
          >
            <span role="cell" className="min-w-0">
              <button
                type="button"
                aria-label={t('zones.open', { zone: z.name })}
                className="truncate font-mono font-medium hover:underline focus-visible:ring-2 focus-visible:ring-ring"
                onClick={() => onOpen(z)}
              >
                {z.name}
              </button>
            </span>
            <span role="cell" className="flex min-w-0 items-center gap-2">
              <DnsHostTile provider={hostKey(z.provider)} size="xs" />
              <span className="truncate">
                {t(`providers.${z.provider as ProviderId}`, {
                  defaultValue: z.provider,
                })}
              </span>
            </span>
            <span role="cell" className="hidden sm:block">
              {z.private ? t('zones.private') : t('zones.public')}
            </span>
            <span role="cell" className="hidden sm:block">
              <Badge
                variant={
                  s === 'active'
                    ? 'success'
                    : s === 'unknown'
                      ? 'muted'
                      : 'warning'
                }
              >
                {t(`zones.${s}`)}
              </Badge>
            </span>
            <span role="cell" className="hidden sm:block">
              {z.record_count ?? t('zones.unknown')}
            </span>
            <span role="cell">
              <DelegationChip zone={z} />
            </span>
          </div>
        )
      })}
    </div>
  )
}
