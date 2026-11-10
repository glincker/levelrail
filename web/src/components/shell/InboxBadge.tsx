import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { InboxGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { CountBadge } from './CountBadge'
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
      className="relative @max-md:size-10"
      aria-label={label}
      title={label}
      render={<Link to="/inbox" />}
      nativeButton={false}
    >
      <InboxGlassIcon size="1.4em" />
      {unread > 0 ? <CountBadge count={unread} tone="neutral" /> : null}
    </Button>
  )
}
