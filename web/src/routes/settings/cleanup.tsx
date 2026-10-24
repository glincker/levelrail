import { createFileRoute, Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { BroomIcon } from '@phosphor-icons/react/dist/ssr'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { cn } from '@/lib/utils'
import { CleanupHistoryTab } from '../../components/cleanup/CleanupHistoryTab'
import { CleanupOverviewTab } from '../../components/cleanup/CleanupOverviewTab'
import { DiskDockerTab } from '../../components/cleanup/DiskDockerTab'

type CleanupTab = 'overview' | 'disk' | 'history'

const TABS: CleanupTab[] = ['overview', 'disk', 'history']

export const Route = createFileRoute('/settings/cleanup')({
  validateSearch: (search: Record<string, unknown>): { tab?: CleanupTab } => {
    const tab = TABS.find((x) => x === search.tab)
    return tab && tab !== 'overview' ? { tab } : {}
  },
  component: CleanupPage,
})

function CleanupPage() {
  const { t } = useTranslation('cleanup')
  const { tab = 'overview' } = Route.useSearch()

  return (
    <PageTemplate
      icon={<BroomIcon />}
      title={t('title')}
      description={t('description')}
      tabs={
        <nav
          aria-label={t('tabs.label')}
          className="flex gap-1 overflow-x-auto border-b border-border"
        >
          {TABS.map((id) => (
            <Link
              key={id}
              to="/settings/cleanup"
              search={id === 'overview' ? {} : { tab: id }}
              aria-current={tab === id ? 'page' : undefined}
              className={cn(
                '-mb-px flex shrink-0 items-center border-b-2 px-3 py-2 text-sm font-medium transition-colors outline-none focus-visible:ring-2 focus-visible:ring-ring/60',
                tab === id
                  ? 'border-foreground text-foreground'
                  : 'border-transparent text-muted-foreground hover:text-foreground',
              )}
            >
              {t(`tabs.${id}`)}
            </Link>
          ))}
        </nav>
      }
    >
      {tab === 'overview' ? <CleanupOverviewTab /> : null}
      {tab === 'disk' ? <DiskDockerTab /> : null}
      {tab === 'history' ? <CleanupHistoryTab /> : null}
    </PageTemplate>
  )
}
