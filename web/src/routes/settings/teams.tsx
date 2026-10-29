import { useState } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { useSuspenseQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { UsersGlassIcon } from '@thesvg/ui-react'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { Badge } from '@/components/ui/badge'
import { Banner } from '@/components/ui/banner'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { EmptyState } from '@/components/ui/empty-state'
import { TableSkeleton } from '@/components/ui/table-skeleton'
import { toast } from '@/components/ui/toast'
import { QuotaCard } from '../../components/teams/QuotaCard'
import { TeamEventsCard } from '../../components/teams/TeamEventsCard'
import { TeamInvitesCard } from '../../components/teams/TeamInvitesCard'
import { TeamMembersCard } from '../../components/teams/TeamMembersCard'
import { TeamRoleBadge } from '../../components/teams/TeamRoleBadge'
import { useIsRoot } from '../../hooks/useIsRoot'
import { teamListQueryOptions, useCreateTeam } from '../../queries/teams'

export const Route = createFileRoute('/settings/teams')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(teamListQueryOptions()),
  component: TeamsSettingsPage,
  pendingComponent: () => <TableSkeleton columnCount={4} />,
})

function CreateTeamForm() {
  const { t } = useTranslation('teams')
  const [name, setName] = useState('')
  const create = useCreateTeam()
  return (
    <form
      className="flex gap-2"
      onSubmit={(e) => {
        e.preventDefault()
        if (name.trim() === '') {
          return
        }
        create.mutate(name.trim(), {
          onSuccess: () => {
            setName('')
            toast.add({ title: t('page.created'), type: 'success' })
          },
          onError: (err) =>
            toast.add({
              title: t('errors.generic', { message: err.message }),
              type: 'error',
            }),
        })
      }}
    >
      <Input
        aria-label={t('page.createName')}
        placeholder={t('page.createName')}
        value={name}
        onChange={(e) => {
          setName(e.target.value)
        }}
      />
      <Button type="submit" disabled={create.isPending}>
        {t('page.createSubmit')}
      </Button>
    </form>
  )
}

function TeamsSettingsPage() {
  const { t } = useTranslation('teams')
  const { data: teams } = useSuspenseQuery(teamListQueryOptions())
  const root = useIsRoot()
  const [selected, setSelected] = useState('')
  const team = teams.find((tm) => tm.id === selected) ?? teams[0]
  const manager =
    team !== undefined &&
    (root ||
      (!team.default && (team.role === 'owner' || team.role === 'admin')))

  const onlyDefault = teams.length === 1 && teams[0]?.default === true

  return (
    <PageTemplate
      icon={<UsersGlassIcon size="1.4em" />}
      eyebrow={t('page.eyebrow')}
      title={t('page.title')}
      description={t('page.description')}
      helpPath="/teams-and-roles"
      helpLabel={t('page.help')}
      status={
        <Badge variant="muted">
          {t('page.count', { count: teams.length })}
        </Badge>
      }
      actions={root ? <CreateTeamForm /> : undefined}
      banners={
        onlyDefault && root ? (
          <Banner variant="info" title={t('page.banner.title')}>
            {t('page.banner.body')}
          </Banner>
        ) : null
      }
    >
      {team === undefined ? (
        <EmptyState
          icon={<UsersGlassIcon className="size-6" />}
          title={t('page.title')}
          description={t('page.empty')}
          className="border-0 bg-transparent py-6"
        />
      ) : (
        <>
          <div
            className="flex flex-wrap gap-2"
            role="tablist"
            aria-label={t('page.pick')}
          >
            {teams.map((tm) => (
              <Button
                key={tm.id}
                type="button"
                role="tab"
                aria-selected={tm.id === team.id}
                variant="outline"
                size="sm"
                className="aria-selected:border-foreground aria-selected:bg-muted"
                onClick={() => {
                  setSelected(tm.id)
                }}
              >
                {tm.default ? t('page.defaultTeam') : tm.name}
                {tm.role ? <TeamRoleBadge role={tm.role} /> : null}
              </Button>
            ))}
          </div>
          {team.default && (
            <p className="text-sm text-muted-foreground">
              {t('page.defaultHint')}
            </p>
          )}
          <TeamMembersCard team={team} root={root} />
          <QuotaCard scope="team" scopeId={team.id} />
          {manager && <TeamInvitesCard team={team} root={root} />}
          {manager && <TeamEventsCard teamId={team.id} />}
        </>
      )}
    </PageTemplate>
  )
}
