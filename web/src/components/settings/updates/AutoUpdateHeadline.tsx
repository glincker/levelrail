import { useTranslation } from 'react-i18next'
import { ClockGlassIcon, TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { UPDATE_STATUS_ID } from '@/lib/updatesAnchors'
import type { UpdateStatus } from '@/queries/updates'
import type { UpgradePolicy } from '@/queries/upgradeSchedule'

const HOUR_MS = 3_600_000
// Decisions the server words as a wait for a human, not a transient hold.
const REVIEW_HINT = /breaking|major|notes|acknowledg/i

function localTime(d: Date): string {
  return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
}

function scrollToStatus() {
  const el = document.getElementById(UPDATE_STATUS_ID)
  el?.scrollIntoView({ behavior: 'smooth', block: 'start' })
  el?.querySelector<HTMLElement>('button')?.focus({ preventScroll: true })
}

/** AutoUpdateHeadline is the one line saying what happens next, or why it is held. */
export function AutoUpdateHeadline({
  policy,
  update,
  now,
}: {
  policy: UpgradePolicy
  update: UpdateStatus | undefined
  now: number
}) {
  const { t } = useTranslation('updates')
  const tag =
    update?.update_available && update.latest_version
      ? update.latest_version
      : null
  const decision =
    tag && policy.decision_tag === tag ? (policy.decision ?? '') : ''
  const held = decision !== '' && !/^scheduled/i.test(decision)
  const win = policy.next_window
  const start = win ? new Date(win.start) : null

  if (held) {
    const review = REVIEW_HINT.test(decision)
    return (
      <div className="flex flex-wrap items-center gap-2">
        <TriangleAlertGlassIcon
          aria-hidden="true"
          className="size-4 shrink-0 text-tone-warning"
        />
        <p role="status" className="min-w-0 flex-1 text-sm">
          {t('quick.headline.held', { tag, reason: decision })}
        </p>
        <Button
          type="button"
          size="sm"
          variant="outline"
          onClick={scrollToStatus}
        >
          {t(review ? 'quick.headline.review' : 'quick.headline.installNow')}
        </Button>
      </div>
    )
  }

  if (!start || Number.isNaN(start.getTime())) return null
  const ms = start.getTime() - now
  const open = win?.active === true || ms <= 0
  const eta =
    ms < HOUR_MS
      ? t('quick.headline.inMinutes', {
          count: Math.max(1, Math.round(ms / 60000)),
        })
      : t('quick.headline.inHours', { count: Math.round(ms / HOUR_MS) })
  const time = localTime(start)
  const day = start.toLocaleDateString([], { weekday: 'short' })
  const key = !tag
    ? 'idle'
    : open
      ? 'now'
      : ms < 24 * HOUR_MS
        ? 'tonight'
        : 'later'
  return (
    <div className="flex flex-wrap items-center gap-x-2 gap-y-0.5">
      <ClockGlassIcon aria-hidden="true" className="size-4 shrink-0" />
      <p role="status" className="text-sm">
        {t(`quick.headline.${key}`, { tag, time, day, eta })}
      </p>
      <span className="text-xs text-muted-foreground">
        {start.toISOString().slice(11, 16)} UTC
      </span>
    </div>
  )
}
