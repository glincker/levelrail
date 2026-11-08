import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CheckGlassIcon,
  CircleAlertGlassIcon,
  CircleCheckGlassIcon,
  CircleStopGlassIcon,
  CopyGlassIcon,
  type UiIcon,
} from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { StatusBadge } from '@/components/ui/status-badge'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import type { Tone } from '@/components/kit/tone'
import { Meter } from '../dashboards/widgets/parts'
import { formatBytes } from '../../lib/format'
import { formatDurationMs } from '../../lib/deployDuration'
import type { AppContainer } from '../../types/appContainers'

const STATE_VIEW: Record<string, { tone: Tone; icon: UiIcon }> = {
  running: { tone: 'success', icon: CircleCheckGlassIcon },
  stopped: { tone: 'neutral', icon: CircleStopGlassIcon },
}
const OTHER_STATE = { tone: 'warning' as Tone, icon: CircleAlertGlassIcon }

const HEALTH_TONE: Record<string, Tone> = {
  healthy: 'success',
  unhealthy: 'danger',
  starting: 'warning',
}

function Unavailable() {
  const { t } = useTranslation('appContainers')
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <span
            tabIndex={0}
            className="cursor-help text-muted-foreground underline decoration-dotted underline-offset-2 outline-none focus-visible:ring-2 focus-visible:ring-ring/60"
          >
            {t('unavailable')}
          </span>
        }
      />
      <TooltipContent>{t('unavailableTip')}</TooltipContent>
    </Tooltip>
  )
}

function Row({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex min-h-7 items-start gap-3 py-1 text-[13px]">
      <dt className="w-20 shrink-0 text-muted-foreground">{label}</dt>
      <dd className="min-w-0 flex-1 break-words">{children}</dd>
    </div>
  )
}

function CopyId({ id }: { id: string }) {
  const { t } = useTranslation('appContainers')
  const [copied, setCopied] = useState(false)
  return (
    <span className="inline-flex items-center gap-1">
      <code className="font-mono text-[13px]">{id}</code>
      <Button
        type="button"
        size="icon-xs"
        variant="ghost"
        aria-label={copied ? t('copied') : t('copyId')}
        onClick={() => {
          void navigator.clipboard.writeText(id).then(() => {
            setCopied(true)
          })
        }}
      >
        {copied ? (
          <CheckGlassIcon className="size-3.5" />
        ) : (
          <CopyGlassIcon className="size-3.5" />
        )}
      </Button>
    </span>
  )
}

function UsageBar({
  label,
  percent,
  text,
}: {
  label: string
  percent: number
  text: string
}) {
  return (
    <div className="flex items-center gap-2">
      <div className="w-24 shrink-0">
        <Meter label={label} percent={Math.min(Math.max(percent, 0), 100)} />
      </div>
      <span className="tabular-nums">{text}</span>
    </div>
  )
}

function Chips({ items, empty }: { items: ReactNode[]; empty: string }) {
  if (items.length === 0) {
    return <span className="text-muted-foreground">{empty}</span>
  }
  return <div className="flex flex-wrap gap-1">{items}</div>
}

// now comes from the query's last fetch time so render stays pure.
export function AppContainerCard({ c, now }: { c: AppContainer; now: number }) {
  const { t } = useTranslation('appContainers')
  const missing = (field: string) => c.unavailable.includes(field)
  const view = STATE_VIEW[c.state] ?? OTHER_STATE
  const stateLabel =
    c.state === 'running' || c.state === 'stopped'
      ? t(`state.${c.state}`)
      : t('state.other', { state: c.state })
  const uptime =
    c.state === 'running' && c.started_at
      ? formatDurationMs(now - new Date(c.started_at).getTime())
      : null
  const stats = c.stats
  const memLimit = stats?.memory_limit_bytes || c.limits.memory_bytes
  const memPercent =
    stats && memLimit > 0 ? (stats.memory_bytes / memLimit) * 100 : 0
  const healthStatus = c.health?.status ?? ''
  const hasLimits = c.limits.cpu > 0 || c.limits.memory_bytes > 0

  return (
    <section
      aria-label={c.name}
      className="rounded-lg border border-border bg-card"
    >
      <header className="flex flex-wrap items-center gap-x-3 gap-y-1 border-b border-border px-3 py-1.5">
        <CopyId id={c.id} />
        <Badge variant="outline" className="shrink-0">
          {t('node')}: {c.node}
        </Badge>
        <StatusBadge tone={view.tone} icon={view.icon} label={stateLabel} />
        <span className="text-[13px] text-muted-foreground">
          {missing('restart_count') || c.restart_count === undefined ? (
            <Unavailable />
          ) : (
            t('restarts', { count: c.restart_count })
          )}
        </span>
        <span className="text-[13px] text-muted-foreground">
          {missing('started_at') ? (
            <Unavailable />
          ) : uptime ? (
            t('uptime', { duration: uptime })
          ) : null}
        </span>
      </header>
      <div className="grid gap-x-8 px-3 py-1.5 md:grid-cols-2">
        <dl className="min-w-0">
          <Row label={t('fields.image')}>
            <code className="font-mono text-xs">{c.image}</code>
          </Row>
          <Row label={t('fields.digest')}>
            {missing('image_digest') || !c.image_digest ? (
              <Unavailable />
            ) : (
              <code className="font-mono text-xs">{c.image_digest}</code>
            )}
          </Row>
          <Row label={t('fields.ports')}>
            <Chips
              empty={t('none')}
              items={c.ports.map((p) => (
                <Badge
                  key={`${p.bind}-${p.host}-${p.container}-${p.proto}`}
                  variant="outline"
                  className="font-mono"
                >
                  {p.bind}:{p.host}:{p.container}/{p.proto}
                </Badge>
              ))}
            />
          </Row>
          <Row label={t('fields.networks')}>
            {missing('networks') ? (
              <Unavailable />
            ) : (
              <Chips
                empty={t('none')}
                items={c.networks.map((n) => (
                  <Badge key={n.name} variant="outline" className="font-mono">
                    {n.name}
                    {n.ip ? ` ${n.ip}` : ''}
                  </Badge>
                ))}
              />
            )}
          </Row>
          <Row label={t('fields.mounts')}>
            <Chips
              empty={t('none')}
              items={c.mounts.map((m) => (
                <Badge
                  key={`${m.source}-${m.target}`}
                  variant="outline"
                  className="font-mono"
                  title={m.type}
                >
                  {m.source} {'>'} {m.target}
                  {m.ro ? ` (${t('readOnly')})` : ''}
                </Badge>
              ))}
            />
          </Row>
        </dl>
        <dl className="min-w-0">
          <Row label={t('fields.limits')}>
            {hasLimits
              ? t('limitsValue', {
                  cpu: c.limits.cpu || '-',
                  memory: c.limits.memory_bytes
                    ? formatBytes(c.limits.memory_bytes)
                    : '-',
                })
              : t('noLimit')}
          </Row>
          <Row label={t('fields.cpu')}>
            {stats ? (
              <UsageBar
                label={t('fields.cpu')}
                percent={stats.cpu_percent}
                text={`${stats.cpu_percent.toFixed(1)}%`}
              />
            ) : c.state === 'running' && missing('stats') ? (
              <Unavailable />
            ) : (
              <span className="text-muted-foreground">-</span>
            )}
          </Row>
          <Row label={t('fields.memory')}>
            {stats ? (
              <UsageBar
                label={t('fields.memory')}
                percent={memPercent}
                text={
                  memLimit > 0
                    ? t('usage', {
                        used: formatBytes(stats.memory_bytes),
                        limit: formatBytes(memLimit),
                      })
                    : formatBytes(stats.memory_bytes)
                }
              />
            ) : c.state === 'running' && missing('stats') ? (
              <Unavailable />
            ) : (
              <span className="text-muted-foreground">-</span>
            )}
          </Row>
          <Row label={t('fields.health')}>
            {missing('health') || !c.health ? (
              <Unavailable />
            ) : (
              <span className="flex flex-wrap items-center gap-2">
                <StatusBadge
                  tone={HEALTH_TONE[healthStatus] ?? 'neutral'}
                  icon={
                    healthStatus === 'healthy'
                      ? CircleCheckGlassIcon
                      : CircleAlertGlassIcon
                  }
                  label={
                    healthStatus === 'healthy'
                      ? t('health.healthy')
                      : healthStatus === 'unhealthy'
                        ? t('health.unhealthy')
                        : healthStatus === 'starting'
                          ? t('health.starting')
                          : t('health.none')
                  }
                />
                {c.health.failing_streak > 0 ? (
                  <span className="text-muted-foreground">
                    {t('health.streak', { count: c.health.failing_streak })}
                  </span>
                ) : null}
              </span>
            )}
          </Row>
        </dl>
      </div>
    </section>
  )
}
