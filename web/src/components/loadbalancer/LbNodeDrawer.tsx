import { useTranslation } from 'react-i18next'
import { ArrowRightGlassIcon, ClockGlassIcon } from '@thesvg/ui-react'
import { RelativeTime, StatusPill, Timeline } from '@/components/kit'
import { Button } from '@/components/ui/button'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import type { LoadBalancerAlgorithm } from '../../queries/appLoadBalancer'
import type {
  AdminState,
  LiveUpstream,
  UpstreamHistory,
} from '../../queries/loadBalancerLive'
import { adminActions, unsupportedHint } from './adminActions'
import { summarizeChecks } from './checkSummary'
import { HealthHistoryStrip } from './HealthHistoryStrip'
import { StateGlyphIcon } from './StateGlyphIcon'
import { formatShare, upstreamShares, upstreamView } from './rollup'

interface Props {
  upstream: LiveUpstream | undefined
  history: UpstreamHistory | undefined
  algorithm: LoadBalancerAlgorithm
  allUpstreams: LiveUpstream[]
  historySupported: boolean
  pending: boolean
  onClose: () => void
  onAdmin: (id: string, state: AdminState) => void
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="text-sm font-medium tabular-nums">{value}</dd>
    </div>
  )
}

export function LbNodeDrawer({
  upstream: u,
  history,
  algorithm,
  allUpstreams,
  historySupported,
  pending,
  onClose,
  onAdmin,
}: Props) {
  const { t } = useTranslation('statePlaceholders')
  const view = u ? upstreamView(u) : undefined
  const share = u ? (upstreamShares(allUpstreams, algorithm).get(u.id) ?? 0) : 0
  const recent = (history?.checks ?? []).slice(-12).reverse()
  return (
    <Sheet open={u !== undefined} onOpenChange={(o) => !o && onClose()}>
      <SheetContent className="overflow-y-auto p-4 sm:max-w-md">
        {u && view ? (
          <>
            <SheetHeader className="p-0">
              <SheetTitle className="font-mono text-sm">
                {u.dial || t('lb.table.replica', { replica: u.replica })}
              </SheetTitle>
              <SheetDescription className="sr-only">
                {t('lb.drawer.description')}
              </SheetDescription>
              <div className="flex flex-wrap items-center gap-2">
                <StatusPill
                  tone={view.tone}
                  label={view.label}
                  icon={<StateGlyphIcon glyph={view.glyph} />}
                />
                <span className="text-xs text-muted-foreground">
                  {view.reason}
                </span>
              </div>
            </SheetHeader>

            <dl className="grid grid-cols-3 gap-3">
              <Stat
                label={t('lb.drawer.trafficShare')}
                value={formatShare(share, algorithm)}
              />
              <Stat
                label={t('lb.drawer.connections')}
                value={String(u.active_connections)}
              />
              <Stat label={t('lb.drawer.failures')} value={String(u.fails)} />
              <Stat
                label={t('lb.drawer.latency')}
                value={u.latency_ms ? `${u.latency_ms}ms` : '-'}
              />
              <Stat label={t('lb.drawer.replica')} value={String(u.replica)} />
              <Stat
                label={t('lb.drawer.node')}
                value={u.node_id || t('lb.drawer.local')}
              />
            </dl>

            {u.last_check ? (
              <p className="flex items-center gap-1.5 text-xs text-muted-foreground">
                <ClockGlassIcon className="size-5" aria-hidden="true" />
                {t('lb.drawer.checked')} <RelativeTime at={u.last_check} live />
              </p>
            ) : null}

            <section className="space-y-2">
              <h3 className="text-(length:--glinr-type-size-section) leading-(--glinr-type-leading-section) font-(weight:--glinr-type-heading-weight)">
                {t('lb.drawer.actions')}
              </h3>
              <ul className="space-y-2">
                {adminActions(u, historySupported).map((a) => (
                  <li
                    key={a.state}
                    className="flex items-center justify-between gap-3"
                  >
                    <span className="min-w-0 text-xs text-muted-foreground">
                      {historySupported ? a.meaning : unsupportedHint()}
                    </span>
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      disabled={a.disabled || pending}
                      onClick={() => onAdmin(u.id, a.state)}
                    >
                      {a.label}
                    </Button>
                  </li>
                ))}
              </ul>
            </section>

            {historySupported ? (
              <>
                <section className="space-y-2">
                  <h3 className="text-(length:--glinr-type-size-section) leading-(--glinr-type-leading-section) font-(weight:--glinr-type-heading-weight)">
                    {t('lb.drawer.recentChecks')}
                  </h3>
                  <HealthHistoryStrip checks={history?.checks ?? []} />
                  <p className="text-xs text-muted-foreground">
                    {summarizeChecks(history?.checks ?? [])}
                  </p>
                  {recent.length > 0 ? (
                    <ul className="space-y-1 text-xs">
                      {recent.slice(0, 5).map((c, i) => (
                        <li
                          key={`${c.at}-${i}`}
                          className="flex justify-between gap-2"
                        >
                          <span>
                            {c.ok
                              ? t('lb.drawer.passed')
                              : c.reason
                                ? t('lb.drawer.failedReason', {
                                    reason: c.reason,
                                  })
                                : t('lb.drawer.failed')}
                          </span>
                          <span className="text-muted-foreground tabular-nums">
                            {c.latency_ms ? `${c.latency_ms}ms` : ''}
                          </span>
                        </li>
                      ))}
                    </ul>
                  ) : null}
                </section>
                <section className="space-y-2">
                  <h3 className="text-(length:--glinr-type-size-section) leading-(--glinr-type-leading-section) font-(weight:--glinr-type-heading-weight)">
                    {t('lb.drawer.stateChanges')}
                  </h3>
                  <Timeline
                    emptyLabel={t('lb.drawer.noChanges')}
                    items={(history?.transitions ?? [])
                      .slice()
                      .reverse()
                      .map((tr, i) => ({
                        id: `${tr.at}-${i}`,
                        at: tr.at,
                        icon: <ArrowRightGlassIcon className="size-5" />,
                        tone:
                          tr.to === 'healthy'
                            ? 'success'
                            : tr.to === 'unhealthy'
                              ? 'danger'
                              : 'warning',
                        title: t('lb.drawer.transition', {
                          from: tr.from,
                          to: tr.to,
                        }),
                        detail: tr.reason,
                      }))}
                  />
                </section>
              </>
            ) : null}
          </>
        ) : null}
      </SheetContent>
    </Sheet>
  )
}
