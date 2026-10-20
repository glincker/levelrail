import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { SquaresFourIcon } from '@phosphor-icons/react/dist/ssr'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { DashboardsPanel } from '@/components/dashboards/DashboardsPanel'

interface DashboardsSearch {
  id?: string
}

export const Route = createFileRoute('/dashboards')({
  validateSearch: (search: Record<string, unknown>): DashboardsSearch =>
    typeof search.id === 'string' && search.id ? { id: search.id } : {},
  component: DashboardsPage,
})

function DashboardsPage() {
  const { t } = useTranslation('dashboards')
  const search = Route.useSearch()
  const navigate = useNavigate({ from: Route.fullPath })
  return (
    <PageTemplate
      title={t('title')}
      description={t('description')}
      icon={<SquaresFourIcon />}
      width="wide"
    >
      <DashboardsPanel
        selectedId={search.id ?? ''}
        onSelect={(id) => void navigate({ search: { id }, replace: true })}
      />
    </PageTemplate>
  )
}
