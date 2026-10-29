import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { UsersGlassIcon } from '@thesvg/ui-react'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { setTeamScope, useTeamScope } from '../../lib/teamScope'
import { projectListQueryOptions } from '../../queries/projects'
import { teamListQueryOptions } from '../../queries/teams'
import { TeamRoleBadge } from './TeamRoleBadge'

const ALL = '__all__'

// Shell team and project switcher. The choice is sent as cookies the server
// uses to narrow every list, so changing it refetches everything.
export function TeamSwitcher() {
  const { t } = useTranslation('teams')
  const qc = useQueryClient()
  const scope = useTeamScope()
  const { data: teams } = useQuery(teamListQueryOptions())
  const { data: projects } = useQuery(projectListQueryOptions())

  if (!teams || teams.length < 2) {
    const only = teams?.[0]
    return only?.role ? <TeamRoleBadge role={only.role} /> : null
  }
  const current = teams.find((tm) => tm.id === scope.team)
  const teamProjects = (projects ?? []).filter(
    (p) => scope.team === '' || p.org_id === scope.team,
  )
  const apply = (team: string, project: string) => {
    setTeamScope({ team, project })
    void qc.invalidateQueries()
  }

  return (
    <div className="flex items-center gap-2">
      <Select
        value={scope.team === '' ? ALL : scope.team}
        onValueChange={(next) => {
          apply(!next || next === ALL ? '' : next, '')
        }}
      >
        <SelectTrigger
          size="sm"
          className="w-40"
          aria-label={t('switcher.team')}
        >
          <UsersGlassIcon size="1.2em" aria-hidden="true" />
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={ALL}>{t('switcher.allTeams')}</SelectItem>
          {teams.map((tm) => (
            <SelectItem key={tm.id} value={tm.id}>
              {tm.default ? t('page.defaultTeam') : tm.name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      {teamProjects.length > 0 && (
        <Select
          value={scope.project === '' ? ALL : scope.project}
          onValueChange={(next) => {
            apply(scope.team, !next || next === ALL ? '' : next)
          }}
        >
          <SelectTrigger
            size="sm"
            className="w-40"
            aria-label={t('switcher.project')}
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL}>{t('switcher.allProjects')}</SelectItem>
            {teamProjects.map((p) => (
              <SelectItem key={p.id} value={p.id}>
                {p.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      )}
      {current?.role ? <TeamRoleBadge role={current.role} /> : null}
    </div>
  )
}
