import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { InboxGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { inboxUnread } from '../../lib/inboxEntries'
import { useInboxEntries } from '../../queries/inboxFeed'

export function InboxBadge() {
  const { t } = useTranslation('inbox', { useSuspense: false })
  const { entries } = useInboxEntries()
  const unread = inboxUnread(entries)
  const label =
    unread > 0 ? t('page.badge', { count: unread }) : t('page.badgeNone')
  return (
    <Button
      variant="outline"
      size="icon-sm"
      className="relative"
      aria-label={label}
      title={label}
      render={<Link to="/inbox" />}
      nativeButton={false}
    >
      <InboxGlassIcon size="1.2em" />
      {unread > 0 ? (
        <span
          aria-hidden="true"
          className="absolute -top-1 -right-1 flex size-3.5 items-center justify-center rounded-full bg-destructive text-[9px] font-semibold text-white"
        >
          {unread > 9 ? '9+' : unread}
        </span>
      ) : null}
    </Button>
  )
}
