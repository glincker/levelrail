import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { TriangleAlertGlassIcon, XGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import {
  announcementBannerQueryOptions,
  useAnnouncementAction,
} from '@/queries/announcements'
import { AnnouncementText } from './AnnouncementText'

// Top-of-shell banner for the critical notice. The API decides whether it
// can be dismissed; the only action is the internal route to Settings > Updates.
export function AnnouncementBanner() {
  const { t } = useTranslation('announcements')
  const { data } = useQuery(announcementBannerQueryOptions())
  const snooze = useAnnouncementAction('snooze')
  const dismiss = useAnnouncementAction('dismiss')
  const item = data?.[0]
  if (!item) return null
  return (
    <div
      role="alert"
      className="flex shrink-0 flex-wrap items-center gap-x-3 gap-y-1 border-b border-tone-danger-border bg-tone-danger-soft px-4 py-2 text-sm text-foreground"
    >
      <TriangleAlertGlassIcon
        aria-hidden="true"
        size="1.4em"
        className="text-tone-danger"
      />
      <div className="min-w-0 flex-1 basis-64">
        <p className="font-medium">
          <span className="sr-only">{t('banner.label')}: </span>
          {item.title}
        </p>
        <AnnouncementText
          text={item.body}
          firstOnly
          className="line-clamp-2 text-muted-foreground"
        />
      </div>
      <Button size="sm" render={<Link to="/settings/updates" />}>
        {t('banner.upgrade')}
      </Button>
      <Button
        type="button"
        size="sm"
        variant="outline"
        disabled={snooze.isPending}
        aria-label={t('banner.snoozeLabel', { title: item.title })}
        onClick={() => snooze.mutate(item.id)}
      >
        {t('banner.snooze')}
      </Button>
      {item.dismissible ? (
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          aria-label={t('page.dismiss')}
          title={t('page.dismiss')}
          disabled={dismiss.isPending}
          onClick={() => dismiss.mutate(item.id)}
        >
          <XGlassIcon size="1.4em" />
        </Button>
      ) : null}
    </div>
  )
}
