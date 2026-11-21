import { useTranslation } from 'react-i18next'
import { Banner } from '@/components/ui/banner'
import { Button } from '@/components/ui/button'
import { useCloudAccountActions } from '../../hooks/useCloudAccountActions'
import { spendBanner, spendLines } from '../../lib/cloudSpend'
import { formatAbsoluteTime } from '../../lib/relativeTime'
import {
  SpendRefreshLimited,
  useCloudSpend,
  useRefreshCloudSpend,
} from '../../queries/cloudAccounts'

function messageOf(e: unknown): string {
  return e instanceof Error ? e.message : String(e)
}

export function CloudSpendCard({ accountId }: Readonly<{ accountId: string }>) {
  const { t, i18n } = useTranslation('cloudAccounts')
  const actions = useCloudAccountActions(accountId)
  const spend = useCloudSpend(accountId, actions.viewSpend)
  const refresh = useRefreshCloudSpend(accountId)
  if (!actions.viewSpend) return null
  const r = spend.data
  const banner = r ? spendBanner(r) : null
  const limited =
    refresh.error instanceof SpendRefreshLimited ? refresh.error : null
  const asOf = r?.data_as_of ?? r?.fetched_at

  return (
    <section
      aria-labelledby="cloud-account-spend"
      className="space-y-2 rounded-lg border border-border p-3"
    >
      <div className="flex flex-wrap items-center gap-2">
        <h3 id="cloud-account-spend" className="text-sm font-semibold">
          {t('spend.title')}
        </h3>
        {r?.estimated ? (
          <span className="rounded-md bg-muted px-1.5 py-0.5 text-xs">
            {t('spend.estimated')}
          </span>
        ) : null}
        <Button
          size="sm"
          variant="outline"
          className="ml-auto"
          disabled={refresh.isPending}
          onClick={() => {
            refresh.mutate()
          }}
        >
          {t('spend.refresh')}
        </Button>
      </div>
      {spend.isPending ? (
        <p role="status" className="text-sm text-muted-foreground">
          {t('section.loading')}
        </p>
      ) : null}
      {spend.isError ? (
        <p role="alert" className="text-sm text-destructive">
          {messageOf(spend.error)}
        </p>
      ) : null}
      {banner && r ? (
        <Banner variant="warning" title={t(`spend.state.${banner}`)}>
          {r.fix_hint ?? t(`spend.hint.${banner}`)}
        </Banner>
      ) : null}
      {r && !banner ? (
        <>
          {r.totals.length === 0 ? (
            <p className="text-sm text-muted-foreground">{t('spend.none')}</p>
          ) : (
            <ul className="flex flex-wrap gap-x-6 gap-y-1">
              {spendLines(r.totals, i18n.language).map((l) => (
                <li key={l.currency} className="text-sm">
                  <span className="text-lg font-semibold tabular-nums">
                    {l.text}
                  </span>{' '}
                  <span className="text-xs text-muted-foreground">
                    {t('spend.monthToDate', { currency: l.currency })}
                  </span>
                </li>
              ))}
            </ul>
          )}
          {r.stale ? (
            <p className="text-xs text-muted-foreground">{t('spend.stale')}</p>
          ) : null}
        </>
      ) : null}
      {asOf ? (
        <p className="text-xs text-muted-foreground">
          {t('spend.dataAge', { time: formatAbsoluteTime(asOf) })}
        </p>
      ) : null}
      {limited ? (
        <p role="status" className="text-xs text-muted-foreground">
          {limited.retryAfterSeconds === null
            ? t('spend.limited')
            : t('spend.limitedIn', { seconds: limited.retryAfterSeconds })}
        </p>
      ) : null}
      {refresh.isError && !limited ? (
        <p role="alert" className="text-xs text-destructive">
          {messageOf(refresh.error)}
        </p>
      ) : null}
    </section>
  )
}
