import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { BellGlassIcon } from '@thesvg/ui-react'
import { ApiErrorAlert } from '@/components/ApiErrorAlert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { formatDate } from '@/lib/format'
import { routeErrorMessage } from '@/lib/apiError'
import {
  announcementsQueryOptions,
  useAnnouncementAction,
  type Announcement,
  type AnnouncementTier,
} from '@/queries/announcements'
import {
  useRuntimeSettings,
  useSetRuntimeSetting,
} from '@/queries/runtimeSettings'
import { AnnouncementText } from './AnnouncementText'
import { KEY_NEWSLETTER } from './AnnouncementsCard'

const TIER_VARIANT = {
  critical: 'destructive',
  advisory: 'warning',
  feature: 'accent',
  maintenance: 'info',
  info: 'info',
  newsletter: 'muted',
} as const satisfies Record<AnnouncementTier, string>

function Item({ a }: { a: Announcement }) {
  const { t } = useTranslation('announcements')
  const read = useAnnouncementAction('read')
  const dismiss = useAnnouncementAction('dismiss')
  const fallback = ''
  const failed = () =>
    toast.add({ title: t('page.actionFailed'), type: 'error' })
  const tier = a.tier in TIER_VARIANT ? a.tier : 'info'
  return (
    <li className="space-y-2 rounded-lg border border-border bg-card p-3 text-sm">
      <div className="flex flex-wrap items-center gap-2">
        <Badge variant={TIER_VARIANT[tier]}>{t(`tier.${tier}`)}</Badge>
        <h3 className="min-w-0 flex-1 font-medium">{a.title}</h3>
        <Badge variant={a.read ? 'muted' : 'outline'}>
          {a.read ? t('page.read') : t('page.unread')}
        </Badge>
      </div>
      <AnnouncementText text={a.body} className="text-muted-foreground" />
      <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-muted-foreground">
        <span>
          {t('page.issued', { date: formatDate(a.issued_at, fallback) })}
        </span>
        <span>
          {t('page.expires', { date: formatDate(a.expires_at, fallback) })}
        </span>
        {a.applies_to ? (
          <span>{t('page.appliesTo', { range: a.applies_to })}</span>
        ) : null}
        {a.snoozed_until ? (
          <span>
            {t('page.snoozed', { date: formatDate(a.snoozed_until, fallback) })}
          </span>
        ) : null}
        {a.dismissed ? <span>{t('page.dismissed')}</span> : null}
        {!a.dismissible ? <span>{t('page.locked')}</span> : null}
        <span className="ms-auto flex gap-2">
          {a.cta ? (
            <Button size="sm" render={<Link to="/settings/updates" />}>
              {t('page.upgrade')}
            </Button>
          ) : null}
          {!a.read ? (
            <Button
              size="sm"
              variant="outline"
              disabled={read.isPending}
              onClick={() => read.mutate(a.id, { onError: failed })}
            >
              {t('page.markRead')}
            </Button>
          ) : null}
          {a.dismissible && !a.dismissed ? (
            <Button
              size="sm"
              variant="ghost"
              disabled={dismiss.isPending}
              onClick={() => dismiss.mutate(a.id, { onError: failed })}
            >
              {t('page.dismiss')}
            </Button>
          ) : null}
        </span>
      </div>
    </li>
  )
}

function List({ items, label }: { items: Announcement[]; label: string }) {
  return (
    <ul aria-label={label} className="space-y-2">
      {items.map((a) => (
        <Item key={a.id} a={a} />
      ))}
    </ul>
  )
}

export function AnnouncementsPage() {
  const { t } = useTranslation('announcements')
  const q = useQuery(announcementsQueryOptions())
  const settings = useRuntimeSettings()
  const save = useSetRuntimeSetting()
  const showNewsletter =
    settings.data?.settings.find((s) => s.key === KEY_NEWSLETTER)?.value !==
    false

  const items = q.data?.items ?? []
  const status = q.data?.status
  const notices = items.filter((a) => a.tier !== 'newsletter')
  const newsletter = items.filter((a) => a.tier === 'newsletter')
  const off = status?.mode === 'off'

  return (
    <PageTemplate
      icon={<BellGlassIcon size="1.4em" />}
      title={t('page.title')}
      description={t('page.description')}
      actions={
        <Button
          size="sm"
          variant="outline"
          render={<Link to="/settings/updates" hash="announcements" />}
        >
          {t('page.settings')}
        </Button>
      }
    >
      {q.isPending ? (
        <Skeleton className="h-32 w-full" aria-hidden="true" />
      ) : q.isError ? (
        <ApiErrorAlert error={q.error} />
      ) : (
        <div className="space-y-6">
          {status?.last_error ? (
            <p role="status" className="text-sm text-tone-warning">
              {t('page.lastError', { message: status.last_error })}
            </p>
          ) : null}
          {status && !status.trust_configured && !off ? (
            <p role="status" className="text-sm text-muted-foreground">
              {t('page.noTrust')}
            </p>
          ) : null}
          {status?.stale && !off ? (
            <p role="status" className="text-sm text-muted-foreground">
              {t('page.stale')}
            </p>
          ) : null}
          {notices.length === 0 && (off || newsletter.length === 0) ? (
            <EmptyState
              compact
              kind="no-results"
              icon={<BellGlassIcon className="size-6" />}
              title={off ? t('page.emptyOffTitle') : t('page.emptyTitle')}
              description={off ? t('page.emptyOffBody') : t('page.emptyBody')}
              action={
                <Button
                  size="sm"
                  render={<Link to="/settings/updates" hash="announcements" />}
                >
                  {t('page.emptyAction')}
                </Button>
              }
            />
          ) : null}
          {notices.length > 0 ? (
            <section aria-labelledby="ann-notices" className="space-y-2">
              <h2 id="ann-notices" className="text-sm font-medium">
                {t('page.notices')}
              </h2>
              <List items={notices} label={t('page.notices')} />
            </section>
          ) : null}
          {!off ? (
            <section
              aria-labelledby="ann-newsletter"
              className="space-y-2 opacity-90"
            >
              <div className="flex flex-wrap items-center justify-between gap-2">
                <div>
                  <h2 id="ann-newsletter" className="text-sm font-medium">
                    {t('page.newsletter')}
                  </h2>
                  <p className="text-xs text-muted-foreground">
                    {t('page.newsletterHint')}
                  </p>
                </div>
                <label className="flex items-center gap-2 text-sm">
                  <Switch
                    size="sm"
                    checked={showNewsletter}
                    disabled={save.isPending}
                    aria-label={t('page.newsletterToggle')}
                    onCheckedChange={(v) =>
                      save.mutate(
                        { key: KEY_NEWSLETTER, value: v },
                        {
                          onError: (e) =>
                            toast.add({
                              title: routeErrorMessage(e),
                              type: 'error',
                            }),
                        },
                      )
                    }
                  />
                  {t('page.newsletterToggle')}
                </label>
              </div>
              {!showNewsletter ? (
                <p className="text-sm text-muted-foreground">
                  {t('page.newsletterHidden')}
                </p>
              ) : newsletter.length === 0 ? (
                <p className="text-sm text-muted-foreground">
                  {t('page.newsletterEmpty')}
                </p>
              ) : (
                <List items={newsletter} label={t('page.newsletter')} />
              )}
            </section>
          ) : null}
        </div>
      )}
    </PageTemplate>
  )
}
