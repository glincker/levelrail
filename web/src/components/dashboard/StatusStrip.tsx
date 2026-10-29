import { useState, type ReactNode } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  ChevronDownGlassIcon,
  CircleAlertGlassIcon,
  CircleCheckGlassIcon,
  ServerGlassIcon,
} from '@thesvg/ui-react'
import { SkeletonLine, type Tone } from '@/components/kit'
import { TONE } from '@/components/kit/tone'
import { cn } from '@/lib/utils'
import type { AttentionLevel } from '../../lib/attentionSummary'
import { causeCount } from '../../lib/dashboardHome'
import { unsnoozeNow } from '../shell/snoozeStore'
import { useNodeListOptional } from '../../queries/nodes'
import { RelativeTime } from '../kit/RelativeTime'
import { useDashboardAttention } from './useDashboardAttention'
import { NoticeRows } from './DashboardBanners'
import { UpdateStrip } from './UpdateStrip'
import { useDashboardNotices } from '../../hooks/useDashboardNotices'

const LEVEL_TONE: Record<AttentionLevel, Tone> = {
  ok: 'success',
  warning: 'warning',
  critical: 'danger',
}

/** Names offline nodes with how long they have been down and links to the first one. */
function OfflineNodes({
  nodes,
}: {
  nodes: Array<{ id: string; name: string; since?: string }>
}) {
  const { t } = useTranslation('dashboard')
  const first = nodes[0]
  if (!first) return null
  const names = nodes.map((n) => n.name).join(', ')
  const since = nodes.find((n) => n.since)?.since
  return (
    <Link
      to="/nodes/$id"
      params={{ id: first.id }}
      className={cn(
        'flex flex-wrap items-center gap-x-1.5 text-sm font-medium hover:underline',
        TONE.danger.text,
      )}
    >
      <ServerGlassIcon aria-hidden="true" className="size-6" />
      {t('strip.nodeOffline', { count: nodes.length, names })}
      {since ? (
        <span className="font-normal">
          {t('strip.nodeOfflineSince')} <RelativeTime at={since} live />
        </span>
      ) : null}
    </Link>
  )
}

/** StatusStrip is the one place the dashboard states fleet health and platform notices, collapsed to a single line. */
export function StatusStrip({
  apps,
  trailing,
}: {
  apps: number
  trailing?: ReactNode
}) {
  const { t } = useTranslation('dashboard')
  const { level, count, visible, snoozed, isLoading } = useDashboardAttention()
  const notices = useDashboardNotices()
  const nodeList = useNodeListOptional()
  const offlineNodes = (nodeList.data ?? [])
    .filter((n) => n.status === 'offline')
    .map((n) => ({
      id: n.id,
      name: n.name,
      since: n.offline_since ?? n.last_seen_at,
    }))
  const nodesDown = offlineNodes.length > 0
  const [open, setOpen] = useState(false)
  const worst: Tone = nodesDown
    ? 'danger'
    : notices.some((n) => n.tone === 'danger')
      ? 'danger'
      : LEVEL_TONE[level]
  const tone =
    !nodesDown && level === 'ok' && notices.length > 0 ? 'warning' : worst
  const causes = causeCount(visible)
  const Icon = tone === 'success' ? CircleCheckGlassIcon : CircleAlertGlassIcon
  const label =
    level === 'ok'
      ? t('strip.ok', { count: apps })
      : causes < count
        ? t('strip.causes', { count, causes })
        : t('strip.issues', { count })
  return (
    <div className="flex flex-col gap-2">
      <div
        className={cn(
          'rounded-lg border px-4 py-2',
          TONE[tone].soft,
          TONE[tone].border,
        )}
      >
        <div
          role="status"
          className="flex flex-wrap items-center gap-x-3 gap-y-2"
        >
          {nodesDown ? (
            <OfflineNodes nodes={offlineNodes} />
          ) : isLoading ? (
            <SkeletonLine width={220} className="h-5" />
          ) : (
            <span
              className={cn(
                'flex items-center gap-2 text-sm font-medium',
                TONE[tone].text,
              )}
            >
              <Icon aria-hidden="true" className="size-6" />
              {label}
            </span>
          )}
          <span className="ml-auto flex flex-wrap items-center gap-2">
            {notices.length > 0 ? (
              <button
                type="button"
                aria-expanded={open}
                onClick={() => {
                  setOpen((v) => !v)
                }}
                className="inline-flex items-center gap-1 rounded-full border border-border px-2.5 py-1 text-xs text-foreground transition-colors hover:bg-muted"
              >
                {t('strip.notices', { count: notices.length })}
                <ChevronDownGlassIcon
                  aria-hidden="true"
                  className={cn(
                    'size-4.5 transition-transform',
                    open && 'rotate-180',
                  )}
                />
              </button>
            ) : null}
            {snoozed.length > 0 ? (
              <button
                type="button"
                onClick={() => {
                  snoozed.forEach((i) => {
                    unsnoozeNow(i.id)
                  })
                }}
                className="text-xs text-muted-foreground underline-offset-2 hover:text-foreground hover:underline"
              >
                {t('strip.showSnoozed', { count: snoozed.length })}
              </button>
            ) : null}
            {level === 'ok' ? null : (
              <Link
                to="/status"
                className="text-xs text-muted-foreground hover:text-foreground"
              >
                {t('strip.details')}
              </Link>
            )}
            {trailing}
          </span>
        </div>
        {open && notices.length > 0 ? (
          <div className="mt-2 border-t border-border pt-2">
            <NoticeRows notices={notices} />
          </div>
        ) : null}
      </div>
      <UpdateStrip />
    </div>
  )
}
