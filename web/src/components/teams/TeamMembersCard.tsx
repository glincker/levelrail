import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  ResponsiveTable,
  type ResponsiveColumn,
} from '@/components/ui/responsive-table'
import { toast } from '@/components/ui/toast'
import { useAuthUsername } from '../../hooks/useAuthUsername'
import {
  TEAM_ROLES,
  teamMembersQueryOptions,
  useRemoveMember,
  useSetMemberRole,
  useTransferTeam,
  type TeamMemberResource,
  type TeamResource,
  type TeamRole,
} from '../../queries/teams'
import { TeamRoleBadge } from './TeamRoleBadge'

function assignable(myRole: TeamResource['role'], root: boolean): TeamRole[] {
  if (root || myRole === 'owner') {
    return TEAM_ROLES
  }
  return myRole === 'admin' ? ['developer', 'viewer'] : []
}

export function TeamMembersCard({
  team,
  root,
}: {
  team: TeamResource
  root: boolean
}) {
  const { t } = useTranslation('teams')
  const me = useAuthUsername()
  const { data: members = [] } = useQuery(teamMembersQueryOptions(team.id))
  const setRole = useSetMemberRole(team.id)
  const remove = useRemoveMember(team.id)
  const transfer = useTransferTeam(team.id)
  const canManage = team.default
    ? root
    : root || team.role === 'owner' || team.role === 'admin'
  const roles = assignable(team.role, root)

  const report = (
    key:
      | 'members.left'
      | 'members.removed'
      | 'members.roleChanged'
      | 'members.transferred',
  ) => ({
    onSuccess: () => toast.add({ title: t(key), type: 'success' }),
    onError: (e: Error) =>
      toast.add({
        title: t('errors.generic', { message: e.message }),
        type: 'error',
      }),
  })

  const isEditable = (m: TeamMemberResource) =>
    canManage && m.email !== me && roles.includes(m.role)

  const columns: ResponsiveColumn<TeamMemberResource>[] = [
    {
      id: 'email',
      header: t('members.email'),
      primary: true,
      cell: (m) => (
        <span className="font-mono text-xs break-all">{m.email}</span>
      ),
    },
    {
      id: 'name',
      header: t('members.name'),
      priority: 3,
      cell: (m) => m.display_name,
    },
    {
      id: 'role',
      header: t('members.role'),
      cell: (m) =>
        isEditable(m) ? (
          <Select
            value={m.role}
            onValueChange={(next) => {
              if (next && next !== m.role) {
                setRole.mutate(
                  { userId: m.user_id, role: next },
                  report('members.roleChanged'),
                )
              }
            }}
          >
            <SelectTrigger
              size="sm"
              className="w-36"
              aria-label={t('members.roleLabel', { email: m.email })}
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {roles.map((r) => (
                <SelectItem key={r} value={r}>
                  {t(`roles.${r}`)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        ) : (
          <TeamRoleBadge role={m.role} />
        ),
    },
  ]

  return (
    <Card>
      <CardHeader className="flex flex-row flex-wrap items-center justify-between">
        <CardTitle>{t('members.title')}</CardTitle>
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={() => {
            remove.mutate('me', report('members.left'))
          }}
        >
          {t('members.leave')}
        </Button>
      </CardHeader>
      <CardContent>
        <ResponsiveTable
          rows={members}
          columns={columns}
          getRowId={(m) => m.user_id}
          ariaLabel={t('members.tableLabel')}
          renderActions={(m) => {
            const editable = isEditable(m)
            const canTransfer =
              team.role === 'owner' && m.email !== me && !team.default
            if (!canTransfer && !editable) return null
            return (
              <span className="flex items-center gap-1">
                {canTransfer ? (
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    onClick={() => {
                      transfer.mutate(m.user_id, report('members.transferred'))
                    }}
                  >
                    {t('members.transfer')}
                  </Button>
                ) : null}
                {editable ? (
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    onClick={() => {
                      remove.mutate(m.user_id, report('members.removed'))
                    }}
                  >
                    {t('members.remove')}
                  </Button>
                ) : null}
              </span>
            )
          }}
        />
      </CardContent>
    </Card>
  )
}
