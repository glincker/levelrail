import { createFileRoute } from '@tanstack/react-router'
import { useSuspenseQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { IdCardLanyardGlassIcon } from '@thesvg/ui-react'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { Button } from '@/components/ui/button'
import { TableSkeleton } from '@/components/ui/table-skeleton'
import { RoleFormDialog } from '../../components/access/RoleFormDialog'
import { RolesTable } from '../../components/access/RolesTable'
import { useIsRoot } from '../../hooks/useIsRoot'
import { requireExperimental } from '../../lib/experimental'
import { roleListQueryOptions } from '../../queries/roles'
import { userListQueryOptions } from '../../queries/users'

export const Route = createFileRoute('/settings/roles')({
  beforeLoad: ({ context: { queryClient }, location }) =>
    requireExperimental(queryClient, 'access-roles', location.href),
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
    <PageTemplate
      icon={<IdCardLanyardGlassIcon size="1.4em" />}
      title={t('roles.title')}
      description={t('roles.description')}
      actions={
        isRoot ? (
          <RoleFormDialog trigger={<Button>{t('roles.create')}</Button>} />
        ) : undefined
      }
    >
      <RolesTable roles={roles} canManage={isRoot} />
    </PageTemplate>
  )
}

function RolesSettingsPending() {
  return (
    <div className="space-y-6" aria-hidden="true">
      <TableSkeleton columnCount={5} />
    </div>
  )
}
