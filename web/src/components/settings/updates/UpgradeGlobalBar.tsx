import '../../../styles/upgradeMotion.css'
import { useEffect } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  CircleCheckGlassIcon,
  RefreshCwGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import { cn } from '@/lib/utils'
import { toast } from '../../ui/toast'
import { useUpgradeStatus } from '../../../hooks/useUpgradeStatus'
import { usePrefersReducedMotion } from '../../../hooks/usePrefersReducedMotion'
import { clearUpgrade } from '../../../lib/upgradeTracker'
import { markSeen } from '../../../lib/upgradeSeen'
import { formatElapsed, useNow } from '../../../lib/upgradeTime'
import { scaleClass } from './progressScale'

const COLLAPSE_MS = 2_500

// Global realtime bar: hidden when idle, never dismissible while running,
// collapses to a toast once the run ends.
export function UpgradeGlobalBar() {
  const { t } = useTranslation('updates')
  const s = useUpgradeStatus()
  const motion = !usePrefersReducedMotion()
  const live = s.state === 'running' || s.state === 'reconnecting'
  const now = useNow(live)
  const raw = s.attempt?.outcome
  const outcome = raw && raw !== 'running' ? raw : undefined
  const finished = s.state === 'finished'

  useEffect(() => {
    if (!finished) return
    const attemptId = s.attempt?.id ?? ''
    const id = window.setTimeout(() => {
      if (attemptId) markSeen('toast', attemptId)
      toast.add({
        title: t(`progress.end.${outcome ?? 'failed'}`, { version: s.target }),
        type:
          outcome === 'succeeded'
            ? 'success'
            : outcome === 'rolled_back'
              ? 'warning'
              : 'error',
      })
      clearUpgrade()
    }, COLLAPSE_MS)
    return () => window.clearTimeout(id)
  }, [finished, outcome, s.target, s.attempt?.id, t])

  if (s.state === 'idle') return null
  const reconnecting = s.state === 'reconnecting'
  const ok = outcome === 'succeeded'
  const amber = finished && !ok
  const percent = finished ? 100 : s.view.percent
  const step = s.view.currentStep || s.lastStep
  const stage = reconnecting
    ? t('pipeline.bar.reconnecting')
    : finished
      ? t(`progress.end.${outcome ?? 'failed'}`, { version: s.target })
      : step
        ? t(`selfUpgrade.step.${step}`, { defaultValue: step })
        : t('progress.waiting')
  const begin = s.startedAt ?? now

  return (
    <div
      className={cn(
        'sticky top-0 z-30 border-b px-4 py-2 text-sm',
        amber
          ? 'border-amber-300 bg-amber-50 text-amber-900 dark:border-amber-900/50 dark:bg-amber-900/20 dark:text-amber-200'
          : 'border-primary/30 bg-primary/5',
        reconnecting && motion && 'upg-breathe',
      )}
    >
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
        {finished ? (
          ok ? (
            <CircleCheckGlassIcon className="size-5 text-success" aria-hidden />
          ) : (
            <TriangleAlertGlassIcon
              className="size-6 text-tone-warning"
              aria-hidden
            />
          )
        ) : (
          <RefreshCwGlassIcon
            className={cn('size-5 text-primary', motion && 'animate-spin')}
            aria-hidden
          />
        )}
        <span className="font-medium">
          {t('pipeline.bar.title', { version: s.target })}
        </span>
        <span
          key={stage}
          className={cn('min-w-0 truncate', motion && 'upg-fade')}
          aria-live="polite"
        >
          {stage}
        </span>
        {!finished && s.view.total > 0 ? (
          <span className="text-xs text-muted-foreground">
            {t('pipeline.bar.step', {
              n: s.view.stepIndex,
              total: s.view.total,
            })}
          </span>
        ) : null}
        <span className="font-mono text-xs text-muted-foreground">
          {formatElapsed(now - begin)}
        </span>
        <Link
          to="/settings/updates"
          className="ml-auto text-xs font-medium text-primary underline underline-offset-2"
        >
          {t('pipeline.bar.view')}
        </Link>
      </div>
      <div
        role="progressbar"
        aria-label={t('pipeline.bar.title', { version: s.target })}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={percent ?? undefined}
        aria-valuetext={stage}
        className="relative mt-1.5 h-1 overflow-hidden rounded-full bg-muted"
      >
        <div
          className={cn(
            'h-full w-full origin-left rounded-full transition-transform duration-500',
            amber ? 'bg-amber-500' : 'bg-primary',
            percent === null
              ? cn(
                  'bg-[repeating-linear-gradient(45deg,var(--primary)_0_8px,transparent_8px_14px)]',
                  motion && 'upg-stripe',
                )
              : scaleClass(percent),
          )}
        />
        {motion && live ? (
          <div className="upg-sheen absolute inset-y-0 left-0 w-1/6 bg-gradient-to-r from-transparent via-white/60 to-transparent" />
        ) : null}
        {motion && finished && ok ? (
          <div className="upg-celebrate absolute inset-0 bg-gradient-to-r from-transparent via-white/80 to-transparent" />
        ) : null}
      </div>
    </div>
  )
}
