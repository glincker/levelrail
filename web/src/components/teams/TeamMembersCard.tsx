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
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { toast } from '@/components/ui/toast'
import { useAuthUsername } from '../../hooks/useAuthUsername'
import {
  TEAM_ROLES,
  teamMembersQueryOptions,
  useRemoveMember,
  useSetMemberRole,
  useTransferTeam,
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

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between">
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
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('members.email')}</TableHead>
              <TableHead>{t('members.name')}</TableHead>
              <TableHead>{t('members.role')}</TableHead>
              <TableHead className="text-right">
                {t('members.actions')}
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {members.map((m) => {
              const editable =
                canManage && m.email !== me && roles.includes(m.role)
              return (
                <TableRow key={m.user_id}>
                  <TableCell className="font-mono text-xs">{m.email}</TableCell>
                  <TableCell>{m.display_name}</TableCell>
                  <TableCell>
                    {editable ? (
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
                          aria-label={t('members.roleLabel', {
                            email: m.email,
                          })}
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
                    )}
                  </TableCell>
                  <TableCell className="space-x-2 text-right">
                    {team.role === 'owner' &&
                    m.email !== me &&
                    !team.default ? (
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        onClick={() => {
                          transfer.mutate(
                            m.user_id,
                            report('members.transferred'),
                          )
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
                  </TableCell>
                </TableRow>
              )
            })}
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  )
}
