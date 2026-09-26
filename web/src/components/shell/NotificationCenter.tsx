import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { BellIcon, CheckCircleIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import {
  Popover,
  PopoverContent,
  PopoverHeader,
  PopoverTitle,
  PopoverTrigger,
} from '@/components/ui/popover'
import { cn } from '@/lib/utils'
import { pollUnlessMissing } from '../../lib/pollUnlessMissing'
import { useActivityEvents } from '../../queries/activity'
import { useDeployApprovalsOptional } from '../../queries/deployApprovals'
import { failedDeploysQueryOptions } from '../../queries/failedDeploys'
import {
  buildNotifications,
  groupNotifications,
  markAllRead,
  unreadCount,
  type ShellNotification,
} from './notifications'
import { setReadIds, useReadIds } from './readStore'

const SEVERITY_DOT: Record<ShellNotification['severity'], string> = {
  critical: 'bg-destructive',
  warning: 'bg-amber-500',
  info: 'bg-sky-500',
}

export function NotificationCenter() {
  const activity = useActivityEvents()
  const approvals = useDeployApprovalsOptional('pending')
  const failed = useQuery({
    ...failedDeploysQueryOptions(),
    retry: false,
    refetchInterval: pollUnlessMissing(30_000),
  })
  const { ids, set } = useReadIds()
  const list = buildNotifications({
    failedDeploys: failed.data,
    approvals: approvals.data,
    activity: activity.events,
  })
  const unread = unreadCount(list, set)
  const groups = groupNotifications(list)

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
              unread > 0
                ? `Notifications: ${String(unread)} unread`
                : 'Notifications: all read'
            }
            title="Notifications"
          />
        }
      >
        <BellIcon />
        {unread > 0 ? (
          <span
            aria-hidden="true"
            className="absolute -top-1 -right-1 flex size-3.5 items-center justify-center rounded-full bg-destructive text-[9px] font-semibold text-white"
          >
            {unread > 9 ? '9+' : unread}
          </span>
        ) : null}
      </PopoverTrigger>
      <PopoverContent align="end" className="w-96">
        <PopoverHeader className="flex-row items-center justify-between">
          <PopoverTitle>Notifications</PopoverTitle>
          <Button
            type="button"
            size="sm"
            variant="ghost"
            disabled={unread === 0}
            onClick={() => setReadIds(markAllRead(new Set(ids), list))}
          >
            Mark all read
          </Button>
        </PopoverHeader>
        {groups.length === 0 ? (
          <div className="flex flex-col items-center gap-1.5 px-2 py-6 text-center">
            <CheckCircleIcon
              aria-hidden="true"
              className="size-6 text-muted-foreground"
            />
            <p className="text-sm text-muted-foreground">
              You are all caught up.
            </p>
          </div>
        ) : (
          <div className="flex max-h-96 flex-col gap-2 overflow-y-auto">
            {groups.map((g) => (
              <section key={g.group} aria-label={g.label}>
                <h3 className="px-2 py-1 text-xs font-medium text-muted-foreground">
                  {g.label}
                </h3>
                <ul>
                  {g.items.map((n) => (
                    <li key={n.id}>
                      <Link
                        to={n.href}
                        onClick={() =>
                          setReadIds(markAllRead(new Set(ids), [n]))
                        }
                        className="flex items-start gap-2.5 rounded-lg px-2 py-2 hover:bg-muted"
                      >
                        <span
                          aria-hidden="true"
                          className={cn(
                            'mt-1.5 size-2 shrink-0 rounded-full',
                            set.has(n.id)
                              ? 'bg-muted-foreground/30'
                              : SEVERITY_DOT[n.severity],
                          )}
                        />
                        <span className="min-w-0 flex-1">
                          <span
                            className={cn(
                              'block truncate text-sm',
                              set.has(n.id)
                                ? 'text-muted-foreground'
                                : 'font-medium',
                            )}
                          >
                            {n.title}
                          </span>
                          <span className="block truncate text-xs text-muted-foreground">
                            {n.detail}
                          </span>
                        </span>
                      </Link>
                    </li>
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
