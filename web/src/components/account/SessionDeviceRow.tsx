import { useTranslation } from 'react-i18next'
import {
  DesktopIcon,
  GlobeHemisphereWestIcon,
  TerminalWindowIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { BrandIcon } from '../BrandIcon'
import { deviceMarks, lastSeenTone } from '../../lib/deviceMarks'
import { cn } from '@/lib/utils'
import type { SessionEntry } from '../../queries/securityCenter'

const TONE_DOT = {
  live: 'bg-emerald-500',
  recent: 'bg-amber-500',
  idle: 'bg-muted-foreground/40',
} as const

function when(iso: string): string {
  return iso ? new Date(iso).toLocaleString() : ''
}

/** SessionDeviceRow shows one signed-in browser with its marks, network and whether it can approve a new sign-in. */
export function SessionDeviceRow({
  session,
  now,
  busy,
  onRevoke,
}: {
  session: SessionEntry
  now: number
  busy: boolean
  onRevoke: () => void
}) {
  const { t } = useTranslation('settings')
  const marks = deviceMarks(session.browser)
  const tone = lastSeenTone(session.last_seen_at, now)
  return (
    <li className="flex items-center gap-3 py-3">
      <span className="relative flex size-10 shrink-0 items-center justify-center rounded-lg bg-muted">
        {marks.browser ? (
          <BrandIcon name={marks.browser} className="size-5" />
        ) : marks.kind === 'cli' ? (
          <TerminalWindowIcon className="size-5" aria-hidden="true" />
        ) : (
          <DesktopIcon className="size-5" aria-hidden="true" />
        )}
        {marks.os && marks.browser ? (
          <span className="absolute -right-1 -bottom-1 flex size-4 items-center justify-center rounded-full bg-background ring-1 ring-border">
            <BrandIcon name={marks.os} className="size-2.5" />
          </span>
        ) : null}
      </span>
      <div className="min-w-0 flex-1 space-y-0.5">
        <p className="flex flex-wrap items-center gap-1.5 text-sm font-medium">
          <span className="truncate">{session.browser}</span>
          {session.current ? (
            <Badge variant="success">{t('accountPage.sessions.current')}</Badge>
          ) : session.can_approve ? (
            <Badge variant="muted">
              {t('accountPage.sessions.canApprove')}
            </Badge>
          ) : null}
        </p>
        <p className="flex flex-wrap items-center gap-x-3 gap-y-0.5 text-xs text-muted-foreground">
          <span className="inline-flex items-center gap-1">
            <GlobeHemisphereWestIcon className="size-3" aria-hidden="true" />
            {session.ip || session.network}
          </span>
          <span className="inline-flex items-center gap-1.5">
            <span
              aria-hidden="true"
              className={cn('size-1.5 rounded-full', TONE_DOT[tone])}
            />
            {t(`accountPage.sessions.tone.${tone}`)}
            <span className="sr-only">{when(session.last_seen_at)}</span>
          </span>
          <span title={when(session.created_at)}>
            {t('accountPage.sessions.signedInAt', {
              when: when(session.created_at),
            })}
          </span>
        </p>
      </div>
      {session.current ? null : (
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={busy}
          onClick={onRevoke}
        >
          {t('accountPage.sessions.revoke')}
        </Button>
      )}
    </li>
  )
}
