import { isProviderConnected } from '../../lib/providerConnectionState'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { isProviderScopeMissing } from '../../lib/providerCapabilities'
import { formatAbsolute } from '../../lib/dateFormat'
import { formatMoney } from '../../lib/usageFormat'
import {
  useProviderSpend,
  useRefreshProviderSpend,
} from '../../queries/providerResources'
import type { SpendState, SpendValue } from '../../types/providerResources'
import { RelativeTime } from '../kit/RelativeTime'
import { SkeletonList } from '../kit/Skeleton'
import { RefreshButton, RefreshFeedback } from './ProviderRefresh'
import { ProviderScopeNotice } from './ProviderScopeNotice'
import type { SectionProps } from './sections'

interface StatProps {
  label: string
  state: SpendState
  needs?: string
  value: SpendValue | null
  currency: string
  detail?: string
  provider: string
}

function money(v: SpendValue, currency: string): string {
  const n = Number(v.value)
  return Number.isFinite(n) ? formatMoney(n, currency) : v.value
}

// The title carries the exact as-of time and source for hover, and the same
// facts are visible text so touch and keyboard users are not left out.
function Stat({
  label,
  state,
  needs,
  value,
  currency,
  detail,
  provider,
}: StatProps) {
  const { t } = useTranslation('cloudProvider')
  const source = value
    ? t(`spend.source.${value.source}`, { defaultValue: value.source })
    : ''
  const asOf = value?.data_as_of
    ? t('spend.asOfTitle', { time: formatAbsolute(new Date(value.data_as_of)) })
    : undefined
  return (
    <div className="min-w-0 space-y-0.5" title={asOf}>
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="text-sm">
        {state === 'scope_missing' ? (
          <span className="text-xs text-tone-warning">
            {needs
              ? t('spend.scopeMissing', { scope: needs })
              : t('spend.scopeMissingGeneric')}{' '}
            <Link
              to="/integrations/cloud/$provider"
              params={{ provider }}
              hash="cp-capabilities"
              className="underline underline-offset-2"
            >
              {t('spend.seeCapabilities')}
            </Link>
          </span>
        ) : state === 'unavailable' || !value ? (
          <span className="text-xs text-muted-foreground">
            {t('spend.unavailable')}
          </span>
        ) : (
          <>
            <span className="font-medium tabular-nums">
              {money(value, currency)}
            </span>
            <span className="block text-xs text-muted-foreground">
              {source}
              {value.data_as_of ? (
                <>
                  {' '}
                  {t('spend.asOf')} <RelativeTime at={value.data_as_of} />
                </>
              ) : null}
              {detail ? ` ${detail}` : ''}
            </span>
          </>
        )}
      </dd>
    </div>
  )
}

export function ProviderSpendSection({
  provider,
  resource,
  labels,
}: SectionProps) {
  const { t } = useTranslation('cloudProvider')
  const connected = resource.has_token
  const spend = useProviderSpend(provider.id, connected)
  const refresh = useRefreshProviderSpend(provider.id)
  const data = refresh.data ?? spend.data
  const failed = spend.error
  const status = failed && 'status' in failed ? failed.status : 0

  // Data here reads the legacy credential, so an account-only provider has nothing to show.
  if (!connected && isProviderConnected(resource)) return null
  let body
  if (!connected) {
    body = (
      <p className="rounded-lg border border-dashed border-border p-3 text-sm text-muted-foreground">
        {t('spend.notConnected', { name: labels.name })}
      </p>
    )
  } else if (spend.isPending) {
    body = <SkeletonList rows={1} />
  } else if (!data) {
    body = isProviderScopeMissing(failed) ? (
      <ProviderScopeNotice error={failed} provider={provider.id} />
    ) : (
      <Alert variant={status === 501 ? 'default' : 'destructive'}>
        <AlertDescription>
          {status === 501
            ? t('spend.unsupported', { name: labels.name })
            : t('spend.loadError', { message: failed?.message ?? '' })}
        </AlertDescription>
      </Alert>
    )
  } else {
    const c = data.currency || 'USD'
    const est = data.estimate
    body = (
      <dl className="grid grid-cols-2 gap-x-4 gap-y-3 rounded-lg border border-border p-3 sm:grid-cols-4">
        <Stat
          label={t('spend.balance')}
          state={data.balance.state}
          needs={data.balance.needs}
          value={data.balance.account_balance}
          currency={c}
          provider={provider.id}
        />
        <Stat
          label={t('spend.monthToDate')}
          state={data.balance.state}
          needs={data.balance.needs}
          value={data.balance.month_to_date_usage}
          currency={c}
          provider={provider.id}
        />
        <Stat
          label={t('spend.lastInvoice')}
          state={data.last_invoice.state}
          needs={data.last_invoice.needs}
          value={data.last_invoice.amount}
          currency={c}
          provider={provider.id}
          detail={
            data.last_invoice.date
              ? formatAbsolute(new Date(data.last_invoice.date))
              : undefined
          }
        />
        <Stat
          label={t('spend.estimate')}
          state={est.state}
          needs={est.needs}
          value={est.monthly_total}
          currency={c}
          provider={provider.id}
          detail={
            est.state === 'ok'
              ? t('spend.priced', {
                  priced: est.priced_count,
                  total: est.resource_count,
                })
              : undefined
          }
        />
      </dl>
    )
  }

  return (
    <section aria-labelledby="cp-spend" className="space-y-2">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
        <h2 id="cp-spend" className="text-sm font-semibold">
          {t('spend.title')}
        </h2>
        <p className="text-xs text-muted-foreground">{t('spend.lagNote')}</p>
        {connected && data ? (
          <span className="ms-auto">
            <RefreshButton
              pending={refresh.isPending}
              onClick={() => refresh.mutate()}
            />
          </span>
        ) : null}
      </div>
      <RefreshFeedback error={refresh.error} />
      {body}
    </section>
  )
}
