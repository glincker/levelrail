import { Link } from '@tanstack/react-router'
import {
  BellIcon,
  CertificateIcon,
  CheckCircleIcon,
  LockKeyIcon,
  WifiSlashIcon,
} from '@phosphor-icons/react/dist/ssr'
import type { Icon } from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'
import {
  Popover,
  PopoverContent,
  PopoverHeader,
  PopoverTitle,
  PopoverTrigger,
} from '@/components/ui/popover'
import {
  useActivityEvents,
  type ActivityEvent,
  type ActivityKind,
} from '../queries/activity'

// Global, always-visible entry point for the bounded activity feed
// queries/activity.ts builds (see that module's own doc comment for
// exactly what is and is not covered). Lives in routes/__root.tsx's
// header, the one piece of chrome every authenticated route shares,
// immediately left of ThemeToggle: the same "compact, icon-forward"
// slot that button already established, mirroring its own
// variant="outline" size="icon-sm" shape so the two sit as a matched
// pair rather than one looking bolted on.

const KIND_ICON: Record<ActivityKind, Icon> = {
  node_offline: WifiSlashIcon,
  node_cordoned: LockKeyIcon,
  cert_expired: CertificateIcon,
  cert_expiring: CertificateIcon,
}

// Text color only, deliberately not a Badge: each row already reads
// top-to-bottom as one item (icon, title, detail), a colored badge
// chip per row would repeat the same severity the icon already
// carries without adding information, the same "don't double up on
// the same signal" reasoning STATE_DOT_CLASS/STATE_BADGE_VARIANT in
// AlertRulesPanel.tsx apply to firing/pending/ok.
const SEVERITY_ICON_CLASS: Record<ActivityEvent['severity'], string> = {
  critical: 'text-destructive',
  warning: 'text-amber-600 dark:text-amber-400',
}

function ActivityRow({ event }: { event: ActivityEvent }) {
  const EventIcon = KIND_ICON[event.kind]
  return (
    <Link
      to={event.href}
      className="flex items-start gap-2.5 rounded-md px-2 py-2 text-left hover:bg-muted"
    >
      <EventIcon
        className={`mt-0.5 size-4 shrink-0 ${SEVERITY_ICON_CLASS[event.severity]}`}
        aria-hidden="true"
      />
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm font-medium text-foreground">
          {event.title}
        </p>
        <p className="truncate text-xs text-muted-foreground">{event.detail}</p>
      </div>
    </Link>
  )
}

export function NotificationBell() {
  const { events, isLoading } = useActivityEvents()
  const criticalCount = events.filter((e) => e.severity === 'critical').length
  const hasEvents = events.length > 0

  return (
    <Popover>
      <PopoverTrigger
        render={
          <Button
            type="button"
            variant="outline"
            size="icon-sm"
            className="relative"
            aria-label={
              hasEvents
                ? `Notifications: ${events.length} to review`
                : 'Notifications: nothing to report'
            }
            title="Notifications"
          />
        }
      >
        <BellIcon />
        {hasEvents ? (
          <span
            aria-hidden="true"
            className={`absolute -top-1 -right-1 flex size-3.5 items-center justify-center rounded-full text-[9px] font-semibold text-white ${
              criticalCount > 0 ? 'bg-destructive' : 'bg-amber-600'
            }`}
          >
            {events.length > 9 ? '9+' : events.length}
          </span>
        ) : null}
      </PopoverTrigger>
      <PopoverContent align="end" className="w-80">
        <PopoverHeader>
          <PopoverTitle>Notifications</PopoverTitle>
        </PopoverHeader>
        {isLoading ? (
          <p className="px-2 py-3 text-sm text-muted-foreground">Loading...</p>
        ) : hasEvents ? (
          <div className="flex flex-col">
            {events.map((event) => (
              <ActivityRow key={event.id} event={event} />
            ))}
          </div>
        ) : (
          <div className="flex flex-col items-center gap-1.5 px-2 py-6 text-center">
            <CheckCircleIcon
              className="size-5 text-muted-foreground"
              aria-hidden="true"
            />
            <p className="text-sm text-muted-foreground">
              Nothing to report. Every node is reachable and no certificate
              needs attention.
            </p>
          </div>
        )}
      </PopoverContent>
    </Popover>
  )
}
