import { ArrowRightIcon, ClockIcon } from '@phosphor-icons/react/dist/ssr'
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
import { adminActions, UNSUPPORTED_HINT } from './adminActions'
import { summarizeChecks } from './checkSummary'
import { HealthHistoryStrip } from './HealthHistoryStrip'
import { StateGlyphIcon } from './StateGlyphIcon'
import { upstreamShares, upstreamView } from './rollup'

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
                {u.dial || `replica ${u.replica}`}
              </SheetTitle>
              <SheetDescription className="sr-only">
                Upstream details and actions
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
              <Stat label="Traffic share" value={`${share}%`} />
              <Stat label="Connections" value={String(u.active_connections)} />
              <Stat label="Failures" value={String(u.fails)} />
              <Stat
                label="Latency"
                value={u.latency_ms ? `${u.latency_ms}ms` : '-'}
              />
              <Stat label="Replica" value={String(u.replica)} />
              <Stat label="Node" value={u.node_id || 'local'} />
            </dl>

            {u.last_check ? (
              <p className="flex items-center gap-1.5 text-xs text-muted-foreground">
                <ClockIcon className="size-3.5" aria-hidden="true" />
                Checked <RelativeTime at={u.last_check} live />
              </p>
            ) : null}

            <section className="space-y-2">
              <h3 className="text-sm font-medium">Actions</h3>
              <ul className="space-y-2">
                {adminActions(u, historySupported).map((a) => (
                  <li
                    key={a.state}
                    className="flex items-center justify-between gap-3"
                  >
                    <span className="min-w-0 text-xs text-muted-foreground">
                      {historySupported ? a.meaning : UNSUPPORTED_HINT}
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
                  <h3 className="text-sm font-medium">Recent checks</h3>
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
                              ? 'Passed'
                              : `Failed${c.reason ? `: ${c.reason}` : ''}`}
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
                  <h3 className="text-sm font-medium">State changes</h3>
                  <Timeline
                    emptyLabel="No changes recorded."
                    items={(history?.transitions ?? [])
                      .slice()
                      .reverse()
                      .map((t, i) => ({
                        id: `${t.at}-${i}`,
                        at: t.at,
                        icon: <ArrowRightIcon className="size-4" />,
                        tone:
                          t.to === 'healthy'
                            ? 'success'
                            : t.to === 'unhealthy'
                              ? 'danger'
                              : 'warning',
                        title: `${t.from} to ${t.to}`,
                        detail: t.reason,
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
