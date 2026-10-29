import { useState } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { useSuspenseQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ClockGlassIcon, SettingsGlassIcon } from '@thesvg/ui-react'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { Banner } from '@/components/ui/banner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import {
  SettingsCardSkeleton,
  SettingsHeaderSkeleton,
} from '@/components/settings/SettingsSkeletons'
import { SessionPolicyForm } from '@/components/settings/SessionPolicyForm'
import type { SessionPolicy } from '../../queries/sessionPolicy'
import {
  effectiveSessionPolicyQueryOptions,
  sessionPoliciesQueryOptions,
} from '../../queries/sessionPolicy'
import type { TeamResource } from '../../queries/teams'
import { teamListQueryOptions } from '../../queries/teams'

// Root only server-side: saving refuses a policy that would block the caller.
export const Route = createFileRoute('/settings/session-policy')({
  loader: ({ context: { queryClient } }) =>
    Promise.all([
      queryClient.ensureQueryData(sessionPoliciesQueryOptions()),
      queryClient.ensureQueryData(effectiveSessionPolicyQueryOptions()),
      queryClient.ensureQueryData(teamListQueryOptions()),
    ]),
  component: SessionPolicyPage,
  pendingComponent: () => (
    <div className="space-y-6" aria-hidden="true">
      <SettingsHeaderSkeleton icon />
      <SettingsCardSkeleton description={false} rows={3} />
    </div>
  ),
})

function Summary({ policy }: { policy?: SessionPolicy }) {
  const { t } = useTranslation('sso')
  if (!policy) {
    return <Badge variant="muted">{t('policy.noPolicy')}</Badge>
  }
  return (
    <span className="flex flex-wrap gap-1">
      {policy.idle_timeout ? (
        <Badge variant="outline">
          {t('policy.summaryIdle', { value: policy.idle_timeout })}
        </Badge>
      ) : null}
      {policy.max_lifetime ? (
        <Badge variant="outline">
          {t('policy.summaryMaxAge', { value: policy.max_lifetime })}
        </Badge>
      ) : null}
      {policy.max_concurrent_sessions > 0 ? (
        <Badge variant="outline">
          {policy.on_limit === 'reject'
            ? t('policy.summarySessionsReject', {
                count: policy.max_concurrent_sessions,
              })
            : t('policy.summarySessionsEvict', {
                count: policy.max_concurrent_sessions,
              })}
        </Badge>
      ) : null}
      {policy.require_mfa ? (
        <Badge variant="outline">{t('policy.summaryMfa')}</Badge>
      ) : null}
      {policy.ip_allowlist.length > 0 ? (
        <Badge variant="outline">
          {t('policy.summaryAllow', { count: policy.ip_allowlist.length })}
        </Badge>
      ) : null}
    </span>
  )
}

function SessionPolicyPage() {
  const { t } = useTranslation('sso')
  const { data: policies } = useSuspenseQuery(sessionPoliciesQueryOptions())
  const { data: effective } = useSuspenseQuery(
    effectiveSessionPolicyQueryOptions(),
  )
  const { data: teams } = useSuspenseQuery(teamListQueryOptions())
  const [editing, setEditing] = useState<TeamResource | null>(null)
  const byTeam = new Map(policies.map((p) => [p.team_id, p]))

  return (
    <PageTemplate
      icon={<ClockGlassIcon size="1.4em" />}
      eyebrow={t('policy.eyebrow')}
      title={t('policy.title')}
      description={t('policy.description')}
      helpPath="/session-policy"
      status={
        <Badge variant={effective.address_allowed ? 'success' : 'destructive'}>
          {t('policy.yourAddress', { address: effective.your_address })}
        </Badge>
      }
      banners={
        <Banner variant="info" title={t('policy.recoveryTitle')}>
          {t('policy.recoveryBody')}
        </Banner>
      }
    >
      <ul className="divide-y divide-border rounded-lg border border-border bg-card px-4">
        {teams.map((team) => (
          <li
            key={team.id}
            className="flex flex-wrap items-center gap-x-3 gap-y-2 py-3"
          >
            <span className="min-w-0 flex-1 truncate text-sm font-medium">
              {team.name}
            </span>
            <Summary policy={byTeam.get(team.id)} />
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => setEditing(team)}
            >
              <SettingsGlassIcon size="1.4em" aria-hidden="true" />
              {t('policy.edit')}
            </Button>
          </li>
        ))}
      </ul>
      <Sheet
        open={editing !== null}
        onOpenChange={(open) => {
          if (!open) setEditing(null)
        }}
      >
        <SheetContent className="data-[side=right]:sm:max-w-lg">
          <SheetHeader className="border-b border-border pr-12">
            <SheetTitle>
              {t('policy.sheetTitle', { team: editing?.name ?? '' })}
            </SheetTitle>
          </SheetHeader>
          <div className="min-h-0 flex-1 overflow-y-auto px-4 pb-4">
            {editing ? (
              <SessionPolicyForm
                key={editing.id}
                teamId={editing.id}
                policy={byTeam.get(editing.id)}
                onDone={() => setEditing(null)}
              />
            ) : null}
          </div>
        </SheetContent>
      </Sheet>
    </PageTemplate>
  )
}
