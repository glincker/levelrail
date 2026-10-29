import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  BellGlassIcon,
  BellOffGlassIcon,
  CircleCheckGlassIcon,
  XGlassIcon,
} from '@thesvg/ui-react'
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
import { useAttentionItems } from '../../queries/attention'
import { useInboxUnreadCount } from '../../queries/inbox'
import { useDismissAttentionItem } from '../../queries/deviceAuth'
import {
  buildNotifications,
  groupNotifications,
  markAllRead,
  unreadCount,
  type ShellNotification,
} from './notifications'
import { setReadIds, useReadIds } from './readStore'
import { snoozeNow, unsnoozeNow, useSnoozed } from './snoozeStore'

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
  const attention = useAttentionItems()
  const dismiss = useDismissAttentionItem()
  const inboxUnread = useInboxUnreadCount()
  const { t } = useTranslation('attention', { useSuspense: false })
  const { t: tInbox } = useTranslation('inbox', { useSuspense: false })
  const { ids, set } = useReadIds()
  const { isSnoozed } = useSnoozed()
  const [showSnoozed, setShowSnoozed] = useState(false)
  const full = buildNotifications({
    failedDeploys: failed.data,
    approvals: approvals.data,
    activity: activity.events,
    attention: attention.items,
  })
  const snoozedSetup = full.filter((n) => n.snoozeId && isSnoozed(n.snoozeId))
  const list = showSnoozed
    ? full
    : full.filter((n) => !n.snoozeId || !isSnoozed(n.snoozeId))
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
        <BellGlassIcon size="1.4em" />
        {unread > 0 ? (
          <span
            aria-hidden="true"
            className="absolute -top-1 -right-1 flex size-3.5 items-center justify-center rounded-full bg-destructive text-[9px] font-semibold text-white"
          >
            {unread > 9 ? '9+' : unread}
          </span>
        ) : null}
      </PopoverTrigger>
      <PopoverContent
        align="end"
        className="w-96 max-w-[calc(100vw-1rem)]"
        glass
      >
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
        {snoozedSetup.length > 0 ? (
          <Button
            type="button"
            size="sm"
            variant="ghost"
            className="self-start"
            aria-pressed={showSnoozed}
            onClick={() => {
              setShowSnoozed((v) => !v)
            }}
          >
            {t('setup.snoozedCount', { count: snoozedSetup.length })}
          </Button>
        ) : null}
        {groups.length === 0 ? (
          <div className="flex flex-col items-center gap-1.5 px-2 py-6 text-center">
            <CircleCheckGlassIcon
              aria-hidden="true"
              className="size-8 text-muted-foreground"
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
                  {g.group === 'setup'
                    ? t('setup.title', { count: g.items.length })
                    : g.label}
                </h3>
                <ul>
                  {g.items.map((n) => (
                    <li key={n.id} className="flex items-start">
                      <Link
                        to={n.href}
                        onClick={() =>
                          setReadIds(markAllRead(new Set(ids), [n]))
                        }
                        className="flex min-w-0 flex-1 items-start gap-2.5 rounded-lg px-2 py-2 hover:bg-muted"
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
                            title={n.title}
                            className={cn(
                              'block truncate text-sm',
                              set.has(n.id)
                                ? 'text-muted-foreground'
                                : 'font-medium',
                            )}
                          >
                            {n.title}
                          </span>
                          <span
                            title={n.detail}
                            className="line-clamp-2 block text-xs break-words text-muted-foreground"
                          >
                            {n.detail}
                          </span>
                        </span>
                      </Link>
                      {n.snoozeId ? (
                        <SetupActions
                          id={n.snoozeId}
                          snoozed={isSnoozed(n.snoozeId)}
                        />
                      ) : null}
                      {n.dismissKey ? (
                        <Button
                          type="button"
                          size="icon-sm"
                          variant="ghost"
                          aria-label={t('deviceLogin.dismiss')}
                          disabled={dismiss.isPending}
                          onClick={() => {
                            if (n.dismissKey) {
                              dismiss.mutate(n.dismissKey)
                            }
                          }}
                        >
                          <XGlassIcon size="1.4em" />
                        </Button>
                      ) : null}
                    </li>
                  ))}
                </ul>
              </section>
            ))}
          </div>
        )}
        <Link
          to="/alerts"
          className="mt-1 flex items-center justify-between rounded-lg px-2 py-2 text-sm hover:bg-muted"
        >
          <span>{tInbox('bell.link')}</span>
          {(inboxUnread.data ?? 0) > 0 ? (
            <span className="text-xs text-muted-foreground">
              {tInbox('bell.unread', { count: inboxUnread.data })}
            </span>
          ) : null}
        </Link>
      </PopoverContent>
    </Popover>
  )
}

function SetupActions({ id, snoozed }: { id: string; snoozed: boolean }) {
  const { t } = useTranslation('attention', { useSuspense: false })
  if (snoozed) {
    return (
      <Button
        type="button"
        size="sm"
        variant="ghost"
        onClick={() => {
          unsnoozeNow(id)
        }}
      >
        {t('setup.restore')}
      </Button>
    )
  }
  return (
    <>
      <Button
        type="button"
        size="icon-sm"
        variant="ghost"
        aria-label={t('setup.snooze')}
        title={t('setup.snooze')}
        onClick={() => {
          snoozeNow(id, '1d')
        }}
      >
        <BellOffGlassIcon size="1.4em" />
      </Button>
      <Button
        type="button"
        size="icon-sm"
        variant="ghost"
        aria-label={t('setup.dismiss')}
        title={t('setup.dismiss')}
        onClick={() => {
          snoozeNow(id, 'forever')
        }}
      >
        <XGlassIcon size="1.4em" />
      </Button>
    </>
  )
}
