import { providerAccounts } from '../../lib/providerConnectionState'
import { AccountResourcesSection } from './ProviderAccountSections'
import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import {
  SegmentedControl,
  type SegmentedOption,
} from '@/components/ui/segmented-control'
import { cn } from '@/lib/utils'
import { isProviderScopeMissing } from '../../lib/providerCapabilities'
import { formatMoney } from '../../lib/usageFormat'
import {
  useProviderResources,
  useRefreshProviderResources,
} from '../../queries/providerResources'
import type { ProviderResource } from '../../types/providerResources'
import { DestroyServerAction } from '../nodes/DestroyServerAction'
import { RelativeTime } from '../kit/RelativeTime'
import { SkeletonList } from '../kit/Skeleton'
import { StatusPill } from '../kit/StatusPill'
import type { Tone } from '../kit/tone'
import { RefreshButton, RefreshFeedback } from './ProviderRefresh'
import { ProviderScopeNotice } from './ProviderScopeNotice'
import type { SectionProps } from './sections'

type Filter = 'all' | 'managed' | 'discovered'

const TH = 'px-2 py-1.5 text-start text-xs font-medium text-muted-foreground'
const TD = 'px-2 py-2 align-middle text-sm'

const STATUS_TONE: Record<string, Tone> = {
  active: 'success',
  running: 'success',
  new: 'info',
  initializing: 'info',
  off: 'neutral',
  stopped: 'neutral',
  archive: 'neutral',
}

function totalByCurrency(rows: readonly ProviderResource[]): string[] {
  const sums = new Map<string, number>()
  for (const r of rows) {
    if (r.estimated_monthly_price == null) continue
    const cur = r.currency || 'USD'
    sums.set(cur, (sums.get(cur) ?? 0) + r.estimated_monthly_price)
  }
  return [...sums].map(([cur, sum]) => formatMoney(sum, cur))
}

function LegacyProviderResourcesSection({
  provider,
  resource,
  labels,
}: SectionProps) {
  const { t } = useTranslation('cloudProvider')
  const connected = resource.has_token
  const list = useProviderResources(provider.id, connected)
  const refresh = useRefreshProviderResources(provider.id)
  const [filter, setFilter] = useState<Filter>('all')

  const data = refresh.data ?? list.data
  const rows = data?.resources ?? []
  const shown = rows.filter(
    (r) =>
      filter === 'all' ||
      (filter === 'managed' ? r.managed_by_levelrail : !r.managed_by_levelrail),
  )
  const options: SegmentedOption<Filter>[] = [
    { value: 'all', label: t('resources.filter.all') },
    { value: 'managed', label: t('resources.filter.managed') },
    { value: 'discovered', label: t('resources.filter.discovered') },
  ]
  const loadError = list.error ?? null
  const scopeError = isProviderScopeMissing(loadError)
    ? loadError
    : isProviderScopeMissing(refresh.error)
      ? refresh.error
      : null
  const totals = totalByCurrency(shown)

  let body
  if (!connected) {
    body = (
      <p className="rounded-lg border border-dashed border-border p-3 text-sm text-muted-foreground">
        {t('resources.notConnected', { name: labels.name })}
      </p>
    )
  } else if (scopeError) {
    body = (
      <div className="rounded-lg border border-dashed border-border p-3 text-sm">
        <p className="text-muted-foreground">
          {t('resources.scopeMissing', { servers: labels.servers })}
        </p>
        <ProviderScopeNotice error={scopeError} provider={provider.id} />
      </div>
    )
  } else if (list.isPending) {
    body = <SkeletonList rows={3} />
  } else if (!data) {
    const status = loadError && 'status' in loadError ? loadError.status : 0
    body = (
      <Alert variant={status === 501 ? 'default' : 'destructive'}>
        <AlertDescription>
          {status === 501
            ? t('resources.unsupported', { name: labels.name })
            : t('resources.loadError', {
                message: loadError?.message ?? '',
              })}
        </AlertDescription>
      </Alert>
    )
  } else if (rows.length === 0) {
    body = (
      <p className="rounded-lg border border-dashed border-border p-3 text-sm text-muted-foreground">
        {t('resources.empty', { servers: labels.servers })}
      </p>
    )
  } else {
    body = (
      <div className="overflow-x-auto rounded-lg border border-border">
        <table
          className="w-full min-w-[48rem] border-collapse"
          aria-label={t('resources.tableLabel', { servers: labels.servers })}
        >
          <thead className="border-b border-border bg-muted/30">
            <tr>
              <th scope="col" className={TH}>
                {t('servers.name')}
              </th>
              <th scope="col" className={TH}>
                {t('servers.state')}
              </th>
              <th scope="col" className={TH}>
                {labels.location}
              </th>
              <th scope="col" className={TH}>
                {labels.size}
              </th>
              <th scope="col" className={TH}>
                {t('servers.ip')}
              </th>
              <th scope="col" className={cn(TH, 'text-end')}>
                {t('resources.monthly')}
              </th>
              <th scope="col" className={TH}>
                {t('resources.origin')}
              </th>
              <th scope="col" className={TH}>
                {t('servers.node')}
              </th>
              <th scope="col" className={TH}>
                <span className="sr-only">{t('servers.actions')}</span>
              </th>
            </tr>
          </thead>
          <tbody className="divide-y divide-border">
            {shown.map((r) => (
              <tr key={r.id}>
                <td className={`${TD} font-medium`}>{r.name}</td>
                <td className={TD}>
                  <StatusPill
                    tone={STATUS_TONE[r.status] ?? 'neutral'}
                    label={t(`resources.status.${r.status}`, {
                      defaultValue: r.status,
                    })}
                    size="sm"
                  />
                </td>
                <td className={TD}>{r.region}</td>
                <td className={TD}>{r.size}</td>
                <td className={`${TD} font-mono text-xs`}>{r.public_ipv4}</td>
                <td className={`${TD} text-end tabular-nums`}>
                  {r.estimated_monthly_price == null
                    ? t('resources.noPrice')
                    : formatMoney(
                        r.estimated_monthly_price,
                        r.currency || 'USD',
                      )}
                </td>
                <td className={TD}>
                  <Badge variant={r.managed_by_levelrail ? 'info' : 'outline'}>
                    {r.managed_by_levelrail
                      ? t('resources.managed')
                      : t('resources.discovered')}
                  </Badge>
                </td>
                <td className={TD}>
                  {r.enrolled_node ? (
                    <Link
                      to="/nodes/$id"
                      params={{ id: r.enrolled_node.id }}
                      aria-label={t('servers.openNode', {
                        name: r.enrolled_node.name,
                      })}
                      className="underline-offset-2 hover:underline"
                    >
                      {r.enrolled_node.name}
                    </Link>
                  ) : (
                    <span className="text-xs text-muted-foreground">
                      {t('servers.noNode')}
                    </span>
                  )}
                </td>
                <td className={`${TD} text-end`}>
                  {r.managed_by_levelrail && r.enrolled_node ? (
                    <DestroyServerAction nodeId={r.enrolled_node.id} />
                  ) : null}
                </td>
              </tr>
            ))}
          </tbody>
          <tfoot className="border-t border-border bg-muted/30">
            <tr>
              <td colSpan={5} className={`${TD} text-xs text-muted-foreground`}>
                {t('resources.total', { count: shown.length })}
              </td>
              <td className={`${TD} text-end font-medium tabular-nums`}>
                {totals.length > 0
                  ? totals.join(' + ')
                  : t('resources.noPrice')}
              </td>
              <td colSpan={3} />
            </tr>
          </tfoot>
        </table>
      </div>
    )
  }

  return (
    <section aria-labelledby="cp-resources" className="space-y-2">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
        <h2 id="cp-resources" className="text-sm font-semibold">
          {t('resources.title', { servers: labels.servers })}
        </h2>
        {data ? (
          <span className="text-xs text-muted-foreground">
            {t('resources.asOf')} <RelativeTime at={data.generated_at} />
          </span>
        ) : null}
        {connected && rows.length > 0 ? (
          <SegmentedControl
            value={filter}
            onValueChange={setFilter}
            options={options}
            aria-label={t('resources.filter.label')}
            className="ms-auto"
          />
        ) : null}
        {connected && !scopeError ? (
          <RefreshButton
            pending={refresh.isPending}
            onClick={() => refresh.mutate()}
          />
        ) : null}
      </div>
      <RefreshFeedback error={refresh.error} />
      {body}
      {data && rows.length > 0 ? (
        <p className="text-xs text-muted-foreground">
          {t('resources.readOnlyNote')}
        </p>
      ) : null}
    </section>
  )
}

// Account-backed data wins whenever a cloud account exists; the credential reads are the fallback.
export function ProviderResourcesSection(props: SectionProps) {
  return providerAccounts(props.resource).length > 0 ? (
    <AccountResourcesSection {...props} />
  ) : (
    <LegacyProviderResourcesSection {...props} />
  )
}
