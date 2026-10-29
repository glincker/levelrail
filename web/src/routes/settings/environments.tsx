import { createFileRoute } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { LayersGlassIcon, PlusGlassIcon } from '@thesvg/ui-react'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { TableSkeleton } from '@/components/ui/table-skeleton'
import { requireExperimental } from '../../lib/experimental'
import { globalEnvironmentsQueryOptions } from '../../queries/globalEnvironments'
import { EnvironmentsTable } from '../../components/environments/EnvironmentsTable'
import { EnvironmentFormDialog } from '../../components/environments/EnvironmentFormDialog'

export const Route = createFileRoute('/settings/environments')({
  beforeLoad: ({ context: { queryClient }, location }) =>
    requireExperimental(queryClient, 'global-environments', location.href),
  component: EnvironmentsSettingsPage,
})

function EnvironmentsSettingsPage() {
  const { t } = useTranslation('environments')
  const { data: environments, isPending } = useQuery(
    globalEnvironmentsQueryOptions(),
  )
  const createTrigger = (
    <Button type="button">
      <PlusGlassIcon size="1.4em" />
      {t('settings.create')}
    </Button>
  )

  return (
    <PageTemplate
      icon={<LayersGlassIcon size="1.4em" />}
      title={t('settings.title')}
      description={t('settings.description')}
      actions={<EnvironmentFormDialog trigger={createTrigger} />}
    >
      {isPending ? (
        <TableSkeleton columnCount={6} />
      ) : environments && environments.length > 0 ? (
        <EnvironmentsTable environments={environments} />
      ) : (
        <EmptyState
          className="gap-2 border-0 bg-transparent py-4"
          icon={<LayersGlassIcon className="size-6" />}
          title={t('settings.empty.title')}
          description={t('settings.empty.description')}
          action={<EnvironmentFormDialog trigger={createTrigger} />}
        />
      )}
    </PageTemplate>
  )
}
