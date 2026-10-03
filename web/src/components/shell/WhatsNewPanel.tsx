import { useQuery } from '@tanstack/react-query'
import { SparkleIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import {
  Popover,
  PopoverContent,
  PopoverHeader,
  PopoverTitle,
  PopoverTrigger,
} from '@/components/ui/popover'
import { changelogQueryOptions } from '../../queries/changelog'
import { setLastSeenVersion, unreadCount, useLastSeenVersion } from './whatsNew'

// Bell-style unread badge, same shape as NotificationCenter's own, but
// a distinct Sparkle glyph so the two popovers read as different
// things at a glance (operational alerts vs. "what changed here").
export function WhatsNewPanel() {
  const { data } = useQuery(changelogQueryOptions())
  const lastSeen = useLastSeenVersion()
  const entries = data?.entries ?? []
  const unread = unreadCount(entries, lastSeen)

  return (
    <Popover
      onOpenChange={(open) => {
        const latest = entries[0]
        if (open && latest) {
          setLastSeenVersion(latest.version)
        }
      }}
    >
      <PopoverTrigger
        render={
          <Button
            type="button"
            variant="outline"
            size="icon-sm"
            className="relative"
            aria-label={
              unread > 0
                ? `What's new: ${String(unread)} unread`
                : "What's new: all caught up"
            }
            title="What's new"
          />
        }
      >
        <SparkleIcon />
        {unread > 0 ? (
          <span
            aria-hidden="true"
            className="absolute -top-1 -right-1 flex size-3.5 items-center justify-center rounded-full bg-primary text-[9px] font-semibold text-primary-foreground"
          >
            {unread > 9 ? '9+' : unread}
          </span>
        ) : null}
      </PopoverTrigger>
      <PopoverContent align="end" className="w-96">
        <PopoverHeader>
          <PopoverTitle>What&apos;s new</PopoverTitle>
        </PopoverHeader>
        {entries.length === 0 ? (
          <p className="px-2 py-6 text-center text-sm text-muted-foreground">
            No release notes available yet.
          </p>
        ) : (
          <div className="flex max-h-96 flex-col gap-3 overflow-y-auto">
            {entries.map((e) => (
              <section key={e.version} className="px-2">
                <h3 className="flex items-baseline justify-between gap-2 text-xs font-medium text-muted-foreground">
                  <span className="font-semibold text-foreground">
                    {e.version}
                  </span>
                  <span>{e.date}</span>
                </h3>
                <ul className="mt-1 list-disc space-y-0.5 pl-4 text-sm">
                  {e.bullets.map((b, i) => (
                    <li key={i}>{b}</li>
                  ))}
                </ul>
              </section>
            ))}
          </div>
        )}
      </PopoverContent>
    </Popover>
  )
}
