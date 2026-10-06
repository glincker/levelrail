import { createFileRoute } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { PlusIcon, StackIcon } from '@phosphor-icons/react/dist/ssr'
import { PageHeader } from '@/components/shell/PageHeader'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { TableSkeleton } from '@/components/ui/table-skeleton'
import { requireExperimental } from '../../lib/experimental'
import { globalEnvironmentsQueryOptions } from '../../queries/globalEnvironments'
import { EnvironmentsTable } from '../../components/environments/EnvironmentsTable'
import { EnvironmentFormDialog } from '../../components/environments/EnvironmentFormDialog'

export const Route = createFileRoute('/settings/environments')({
  beforeLoad: ({ context: { queryClient } }) =>
    requireExperimental(queryClient, 'global-environments'),
  component: EnvironmentsSettingsPage,
})

function EnvironmentsSettingsPage() {
  const { t } = useTranslation('environments')
  const { data: environments, isPending } = useQuery(
    globalEnvironmentsQueryOptions(),
  )
  const createTrigger = (
    <Button type="button">
      <PlusIcon />
      {t('settings.create')}
    </Button>
  )

  return (
    <div className="space-y-6">
      <PageHeader
        title={t('settings.title')}
        description={t('settings.description')}
        actions={<EnvironmentFormDialog trigger={createTrigger} />}
      />
      {isPending ? (
        <TableSkeleton columnCount={6} />
      ) : environments && environments.length > 0 ? (
        <EnvironmentsTable environments={environments} />
      ) : (
        <EmptyState
          icon={<StackIcon className="size-5" />}
          title={t('settings.empty.title')}
          description={t('settings.empty.description')}
          action={<EnvironmentFormDialog trigger={createTrigger} />}
        />
      )}
    </div>
  )
}
