import { useTranslation } from 'react-i18next'
import { IdCardLanyardGlassIcon, PencilGlassIcon } from '@thesvg/ui-react'
import { ResponsiveTable, type ResponsiveColumn } from '../ui/responsive-table'
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
        className="gap-2 border-0 bg-transparent py-4"
        icon={<IdCardLanyardGlassIcon className="size-5.5" />}
        title={t('roles.empty.title')}
        description={t('roles.empty.description')}
      />
    )
  }

  const columns: ResponsiveColumn<RoleResource>[] = [
    {
      id: 'name',
      primary: true,
      header: t('roles.table.name'),
      cell: (role) => (
        <>
          <span className="flex flex-wrap items-center gap-2">
            <span className="font-medium text-foreground">{role.name}</span>
            {role.builtin ? (
              <Badge variant="outline">{t('roles.badge.builtin')}</Badge>
            ) : null}
          </span>
          <p className="text-xs font-normal text-muted-foreground">
            {role.description}
          </p>
        </>
      ),
    },
    {
      id: 'abilities',
      header: t('roles.table.abilities'),
      priority: 2,
      cell: (role) => (
        <div className="flex flex-wrap gap-1">
          {role.abilities.map((ability) => (
            <Badge key={ability} variant={ABILITY_BADGE_VARIANT[ability]}>
              {ability}
            </Badge>
          ))}
        </div>
      ),
    },
    {
      id: 'visibility',
      header: t('roles.table.visibility'),
      priority: 3,
      cell: (role) => (
        <Badge variant={role.visibility === 'granted' ? 'warning' : 'muted'}>
          {role.visibility === 'granted'
            ? t('roles.badge.visibilityGranted')
            : t('roles.badge.visibilityAll')}
        </Badge>
      ),
    },
    {
      id: 'users',
      header: t('roles.table.users'),
      priority: 2,
      cell: (role) => role.user_count ?? 0,
    },
  ]

  return (
    <ResponsiveTable
      rows={roles}
      columns={columns}
      getRowId={(role) => role.id ?? role.name}
      ariaLabel={t('roles.table.label')}
      renderActions={
        canManage
          ? (role) =>
              role.builtin ? null : (
                <div className="flex flex-wrap items-center justify-end gap-2">
                  <RoleFormDialog
                    role={role}
                    trigger={
                      <Button variant="outline" size="sm">
                        <PencilGlassIcon size="1.2em" />
                        {t('roles.table.edit')}
                      </Button>
                    }
                  />
                  <DeleteRoleDialog role={role} />
                </div>
              )
          : undefined
      }
    />
  )
}
