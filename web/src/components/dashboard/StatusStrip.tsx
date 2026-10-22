import { useState, type ReactNode } from 'react'
import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  ArrowUpIcon,
  CaretDownIcon,
  CheckCircleIcon,
  WarningCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import { SkeletonLine, type Tone } from '@/components/kit'
import { TONE } from '@/components/kit/tone'
import { cn } from '@/lib/utils'
import type { AttentionLevel } from '../../lib/attentionSummary'
import { causeCount } from '../../lib/dashboardHome'
import { useIsRoot } from '../../hooks/useIsRoot'
import { updatesQueryOptions } from '../../queries/updates'
import { unsnoozeNow } from '../shell/snoozeStore'
import { useAttentionSummary } from '../shell/useAttentionSummary'
import { NoticeRows } from './DashboardBanners'
import { useDashboardNotices } from '../../hooks/useDashboardNotices'

const LEVEL_TONE: Record<AttentionLevel, Tone> = {
  ok: 'success',
  warning: 'warning',
  critical: 'danger',
}

function UpdateChip() {
  const { t } = useTranslation('dashboard')
  const isRoot = useIsRoot()
  const updates = useQuery({
    ...updatesQueryOptions(),
    retry: false,
    staleTime: 60 * 60_000,
    enabled: isRoot,
  })
  if (!updates.data?.update_available) return null
  return (
    <Link
      to="/settings/updates"
      className="inline-flex items-center gap-1 rounded-full border border-border px-2.5 py-1 text-xs text-muted-foreground transition-colors hover:text-foreground"
    >
      <ArrowUpIcon aria-hidden="true" className="size-3" />
      {t('strip.update', { version: updates.data.latest_version ?? '' })}
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
  const { level, count, visible, snoozed, isLoading } = useAttentionSummary()
  const notices = useDashboardNotices()
  const [open, setOpen] = useState(false)
  const worst: Tone = notices.some((n) => n.tone === 'danger')
    ? 'danger'
    : LEVEL_TONE[level]
  const tone = level === 'ok' && notices.length > 0 ? 'warning' : worst
  const causes = causeCount(visible)
  const Icon = tone === 'success' ? CheckCircleIcon : WarningCircleIcon
  const label =
    level === 'ok'
      ? t('strip.ok', { count: apps })
      : causes < count
        ? t('strip.causes', { count, causes })
        : t('strip.issues', { count })
  return (
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
        {isLoading ? (
          <SkeletonLine width={220} className="h-5" />
        ) : (
          <span
            className={cn(
              'flex items-center gap-2 text-sm font-medium',
              TONE[tone].text,
            )}
          >
            <Icon weight="fill" aria-hidden="true" className="size-5" />
            {label}
          </span>
        )}
        <span className="ml-auto flex flex-wrap items-center gap-2">
          <UpdateChip />
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
              <CaretDownIcon
                aria-hidden="true"
                className={cn(
                  'size-3 transition-transform',
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
  )
}
