import { useTranslation } from 'react-i18next'
import {
  IdentificationBadgeIcon,
  PencilSimpleIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '../ui/table'
import { Badge } from '../ui/badge'
import { Button } from '../ui/button'
import { EmptyState } from '../ui/empty-state'
import { DeleteRoleDialog } from './DeleteRoleDialog'
import { RoleFormDialog } from './RoleFormDialog'
import { ABILITY_BADGE_VARIANT } from '../../types/token'
import type { RoleResource } from '../../queries/roles'

export function RolesTable({
  roles,
  canManage,
}: {
  roles: RoleResource[]
  canManage: boolean
}) {
  const { t } = useTranslation('access')

  if (roles.length === 0) {
    return (
      <EmptyState
        icon={<IdentificationBadgeIcon className="size-5" />}
        title={t('roles.empty.title')}
        description={t('roles.empty.description')}
      />
    )
  }

  return (
    <div className="rounded-lg border border-border">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('roles.table.name')}</TableHead>
            <TableHead>{t('roles.table.abilities')}</TableHead>
            <TableHead>{t('roles.table.visibility')}</TableHead>
            <TableHead>{t('roles.table.users')}</TableHead>
            <TableHead />
          </TableRow>
        </TableHeader>
        <TableBody>
          {roles.map((role) => (
            <TableRow key={role.id ?? role.name}>
              <TableCell>
                <div className="flex items-center gap-2">
                  <span className="font-medium text-foreground">
                    {role.name}
                  </span>
                  {role.builtin ? (
                    <Badge variant="outline">{t('roles.badge.builtin')}</Badge>
                  ) : null}
                </div>
                <p className="text-xs text-muted-foreground">
                  {role.description}
                </p>
              </TableCell>
              <TableCell>
                <div className="flex flex-wrap gap-1">
                  {role.abilities.map((ability) => (
                    <Badge
                      key={ability}
                      variant={ABILITY_BADGE_VARIANT[ability]}
                    >
                      {ability}
                    </Badge>
                  ))}
                </div>
              </TableCell>
              <TableCell>
                <Badge
                  variant={role.visibility === 'granted' ? 'warning' : 'muted'}
                >
                  {role.visibility === 'granted'
                    ? t('roles.badge.visibilityGranted')
                    : t('roles.badge.visibilityAll')}
                </Badge>
              </TableCell>
              <TableCell>{role.user_count ?? 0}</TableCell>
              <TableCell className="text-right">
                {canManage && !role.builtin ? (
                  <div className="flex items-center justify-end gap-2">
                    <RoleFormDialog
                      role={role}
                      trigger={
                        <Button variant="outline" size="sm">
                          <PencilSimpleIcon />
                          {t('roles.table.edit')}
                        </Button>
                      }
                    />
                    <DeleteRoleDialog role={role} />
                  </div>
                ) : null}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}
