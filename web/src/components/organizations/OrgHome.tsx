import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  BoxGlassIcon,
  ClockGlassIcon,
  CopyGlassIcon,
  DatabaseGlassIcon,
  FolderGlassIcon,
  LayersGlassIcon,
  MailGlassIcon,
  UsersGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Tile, TileGrid, Timeline, type TimelineItem } from '@/components/kit'
import type { OrganizationResource } from '../../types/organization'
import { teamMembersQueryOptions } from '../../queries/teams'
import {
  ORG_ACTIVITY_PAGE,
  orgSummaryQueryOptions,
  useOrgActivity,
  type OrgActivityItem,
} from '../../queries/organizationProfile'
import { OrgAvatar } from './OrgAvatar'
import { orgInitials } from './orgSlug'

const MEMBER_STACK_MAX = 5

function CountLink({
  count,
  to,
  label,
}: {
  count: number
  to: '/projects' | '/settings/teams'
  label: string
}) {
  return (
    <div className="flex items-baseline justify-between gap-2">
      <span className="text-3xl font-semibold tabular-nums">{count}</span>
      <Link
        to={to}
        className="rounded text-sm underline focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
      >
        {label}
      </Link>
    </div>
  )
}

function MemberStack({ orgId }: { orgId: string }) {
  const { data } = useQuery(teamMembersQueryOptions(orgId))
  const shown = (data ?? []).slice(0, MEMBER_STACK_MAX)
  if (shown.length === 0) return null
  return (
    <ul className="flex -space-x-1.5" aria-hidden="true">
      {shown.map((m) => (
        <li
          key={m.user_id}
          className="inline-flex size-7 items-center justify-center rounded-full bg-muted text-[10px] font-semibold ring-2 ring-card"
        >
          {orgInitials(m.display_name || m.email)}
        </li>
      ))}
    </ul>
  )
}

function toTimeline(
  items: OrgActivityItem[],
  label: (a: string) => string,
): TimelineItem[] {
  return items.map((e) => ({
    id: e.id,
    at: e.created_at,
    icon: <ClockGlassIcon aria-hidden="true" />,
    title: label(e.action),
    detail: [e.subject, e.detail].filter(Boolean).join(' '),
    actor: e.actor_name,
  }))
}

export interface OrgHomeProps {
  organization: OrganizationResource
}

export function OrgHome({ organization }: OrgHomeProps) {
  const { t } = useTranslation('settings')
  const summary = useQuery(orgSummaryQueryOptions(organization.id))
  const [more, setMore] = useState(false)
  const activity = useOrgActivity(organization.id, more)
  const [copied, setCopied] = useState(false)

  const data = summary.data
  const counts = data?.counts
  const loaded = data !== undefined
  const failing = data?.failing_apps.names ?? []

  const seen = new Set<string>()
  const merged: OrgActivityItem[] = []
  for (const e of [
    ...(data?.activity ?? []),
    ...(activity.data?.pages.flatMap((p) => p.items) ?? []),
  ]) {
    if (!seen.has(e.id)) {
      seen.add(e.id)
      merged.push(e)
    }
  }
  const canShowMore = more
    ? activity.hasNextPage
    : (data?.activity.length ?? 0) >= ORG_ACTIVITY_PAGE

  const label = (a: string) =>
    t(`organizationHome.actions.${a.replaceAll('.', '_')}`, { defaultValue: a })
  const retry = () => void summary.refetch()
  const err = {
    error: summary.isError,
    errorLabel: t('organizationHome.loadFailed'),
    onRetry: retry,
  }

  function copySlug() {
    if (!organization.slug) return
    void navigator.clipboard.writeText(organization.slug).then(() => {
      setCopied(true)
      window.setTimeout(() => setCopied(false), 1500)
    })
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-4 rounded-xl bg-card p-(--glinr-card-pad) shadow-card ring-1 ring-foreground/10">
        <OrgAvatar
          id={organization.id}
          name={organization.name}
          avatarUrl={organization.avatar_url}
          size="lg"
        />
        <div className="min-w-0 flex-1">
          <h2 className="truncate text-lg font-semibold">
            {organization.name}
          </h2>
          <div className="flex flex-wrap items-center gap-2 text-sm text-muted-foreground">
            {organization.slug ? (
              <>
                <code className="rounded bg-muted px-1.5 py-0.5 text-xs">
                  {organization.slug}
                </code>
                <Button
                  type="button"
                  size="sm"
                  variant="ghost"
                  onClick={copySlug}
                >
                  <CopyGlassIcon size="1.2em" aria-hidden="true" />
                  {copied
                    ? t('organizationHome.copied')
                    : t('organizationHome.copySlug')}
                </Button>
              </>
            ) : (
              <span>{t('organizationHome.noSlug')}</span>
            )}
          </div>
          {organization.description ? (
            <p className="mt-1 text-sm">{organization.description}</p>
          ) : null}
        </div>
      </div>

      <TileGrid>
        <Tile
          title={t('organizationHome.members')}
          size="S"
          icon={<UsersGlassIcon />}
          loading={summary.isPending}
          empty={loaded && counts?.members === 0}
          hideWhenEmpty
          {...err}
        >
          <div className="space-y-2">
            <MemberStack orgId={organization.id} />
            <CountLink
              count={counts?.members ?? 0}
              to="/settings/teams"
              label={t('organizationHome.viewMembers')}
            />
          </div>
        </Tile>
        <Tile
          title={t('organizationHome.projects')}
          size="S"
          icon={<FolderGlassIcon />}
          loading={summary.isPending}
          empty={loaded && counts?.projects === 0}
          hideWhenEmpty
          {...err}
        >
          <CountLink
            count={counts?.projects ?? 0}
            to="/projects"
            label={t('organizationHome.viewProjects')}
          />
        </Tile>
        <Tile
          title={t('organizationHome.environments')}
          size="S"
          icon={<LayersGlassIcon />}
          loading={summary.isPending}
          empty={loaded && counts?.environments === 0}
          hideWhenEmpty
          {...err}
        >
          <span className="text-3xl font-semibold tabular-nums">
            {counts?.environments ?? 0}
          </span>
        </Tile>
        <Tile
          title={t('organizationHome.pendingInvites')}
          size="S"
          icon={<MailGlassIcon />}
          loading={summary.isPending}
          empty={loaded && counts?.pending_invites === 0}
          hideWhenEmpty
          {...err}
        >
          <CountLink
            count={counts?.pending_invites ?? 0}
            to="/settings/teams"
            label={t('organizationHome.viewInvites')}
          />
        </Tile>
        <Tile
          title={t('organizationHome.workloads')}
          size="M"
          icon={<BoxGlassIcon />}
          loading={summary.isPending}
          empty={loaded && counts?.apps === 0 && counts?.databases === 0}
          hideWhenEmpty
          {...err}
        >
          <div className="space-y-3">
            <div className="flex flex-wrap gap-6">
              <span className="flex items-center gap-2">
                <BoxGlassIcon size="1.3em" aria-hidden="true" />
                <span className="text-2xl font-semibold tabular-nums">
                  {counts?.apps ?? 0}
                </span>
                {t('organizationHome.apps')}
              </span>
              <span className="flex items-center gap-2">
                <DatabaseGlassIcon size="1.3em" aria-hidden="true" />
                <span className="text-2xl font-semibold tabular-nums">
                  {counts?.databases ?? 0}
                </span>
                {t('organizationHome.databases')}
              </span>
            </div>
            {failing.length > 0 ? (
              <div>
                <p className="text-xs font-medium text-tone-danger-fg">
                  {t('organizationHome.failing', {
                    count: data?.failing_apps.count ?? failing.length,
                  })}
                </p>
                <ul className="mt-1 flex flex-wrap gap-1.5">
                  {failing.map((name) => (
                    <li key={name}>
                      <Link
                        to="/apps/$name/overview"
                        params={{ name }}
                        className="rounded border border-tone-danger-border bg-tone-danger-soft px-2 py-0.5 text-xs text-tone-danger-fg focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
                      >
                        {name}
                      </Link>
                    </li>
                  ))}
                </ul>
              </div>
            ) : null}
          </div>
        </Tile>
        <Tile
          title={t('organizationHome.activity')}
          size="L"
          icon={<ClockGlassIcon />}
          loading={summary.isPending}
          empty={loaded && merged.length === 0}
          hideWhenEmpty
          {...err}
          footer={
            canShowMore ? (
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={activity.isFetching}
                onClick={() => {
                  if (more) void activity.fetchNextPage()
                  else setMore(true)
                }}
              >
                {t('organizationHome.showMore')}
              </Button>
            ) : undefined
          }
        >
          <Timeline items={toTimeline(merged, label)} />
        </Tile>
      </TileGrid>
    </div>
  )
}
