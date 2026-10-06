import { createFileRoute } from '@tanstack/react-router'
import { useSuspenseQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { IdentificationBadgeIcon } from '@phosphor-icons/react/dist/ssr'
import { PageHeader } from '@/components/shell/PageHeader'
import { Button } from '@/components/ui/button'
import { TableSkeleton } from '@/components/ui/table-skeleton'
import { RoleFormDialog } from '../../components/access/RoleFormDialog'
import { RolesTable } from '../../components/access/RolesTable'
import { useIsRoot } from '../../hooks/useIsRoot'
import { requireExperimental } from '../../lib/experimental'
import { roleListQueryOptions } from '../../queries/roles'
import { userListQueryOptions } from '../../queries/users'

export const Route = createFileRoute('/settings/roles')({
  beforeLoad: ({ context: { queryClient } }) =>
    requireExperimental(queryClient, 'access-roles'),
  loader: ({ context: { queryClient } }) =>
    Promise.all([
      queryClient.ensureQueryData(roleListQueryOptions()),
      queryClient.ensureQueryData(userListQueryOptions()),
    ]),
  component: RolesSettingsPage,
  pendingComponent: RolesSettingsPending,
})

function RolesSettingsPage() {
  const { t } = useTranslation('access')
  const { data: roles } = useSuspenseQuery(roleListQueryOptions())
  const isRoot = useIsRoot()

  return (
    <div className="space-y-6">
      <div className="flex items-start gap-3">
        <div className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
          <IdentificationBadgeIcon className="size-4" />
        </div>
        <div className="min-w-0 flex-1">
          <PageHeader
            title={t('roles.title')}
            description={t('roles.description')}
            actions={
              isRoot ? (
                <RoleFormDialog
                  trigger={<Button>{t('roles.create')}</Button>}
                />
              ) : undefined
            }
          />
        </div>
      </div>
      <RolesTable roles={roles} canManage={isRoot} />
    </div>
  )
}

function RolesSettingsPending() {
  return (
    <div className="space-y-6" aria-hidden="true">
      <TableSkeleton columnCount={5} />
    </div>
  )
}
