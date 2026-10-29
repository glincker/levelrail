import { lazy, Suspense, useState } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { ArrowLeftRightGlassIcon } from '@thesvg/ui-react'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { Banner } from '@/components/ui/banner'
import { EmptyState } from '@/components/ui/empty-state'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { MigrateChecklistCard } from '../components/migrate/MigrateChecklistCard'
import { MigratePlan } from '../components/migrate/MigratePlan'
import { MigrateVerifyCard } from '../components/migrate/MigrateVerifyCard'
import { PortTakeoverPanel } from '../components/PortTakeoverPanel'
import { useMigrateChecklists } from '../queries/migrateWizard'

const MigrateAgentPanel = lazy(() =>
  import('../components/migrate/MigrateAgentPanel').then((m) => ({
    default: m.MigrateAgentPanel,
  })),
)

export const Route = createFileRoute('/migrate')({
  component: MigratePage,
})

function CutoverTab() {
  const { t } = useTranslation('migrate')
  const q = useMigrateChecklists()
  if (q.isPending) return <Skeleton className="h-40 w-full rounded-lg" />
  if (q.isError) {
    return (
      <Banner variant="critical" title={t('cutover.loadError')}>
        {q.error.message}
      </Banner>
    )
  }
  if (q.data.length === 0) {
    return (
      <EmptyState
        icon={<ArrowLeftRightGlassIcon className="size-5.5" />}
        title={t('cutover.empty')}
        description={t('cutover.emptyHint')}
      />
    )
  }
  return (
    <div className="space-y-4">
      {q.data.map((c) => (
        <MigrateChecklistCard key={c.app} checklist={c} />
      ))}
    </div>
  )
}

function MigratePage() {
  const { t } = useTranslation('migrate')
  const [tab, setTab] = useState<string>('plan')
  return (
    <PageTemplate
      icon={<ArrowLeftRightGlassIcon size="1.2em" />}
      title={t('title')}
      description={t('description')}
    >
      <Tabs value={tab} onValueChange={(v) => setTab(String(v))}>
        <TabsList>
          <TabsTrigger value="plan">{t('tabs.plan')}</TabsTrigger>
          <TabsTrigger value="cutover">{t('tabs.cutover')}</TabsTrigger>
          <TabsTrigger value="agent">{t('tabs.agent')}</TabsTrigger>
          <TabsTrigger value="takeover">{t('tabs.takeover')}</TabsTrigger>
        </TabsList>
        <TabsContent value="plan">
          <MigratePlan onOpenCutover={() => setTab('cutover')} />
        </TabsContent>
        <TabsContent value="cutover">
          <div className="space-y-4">
            <MigrateVerifyCard />
            <CutoverTab />
          </div>
        </TabsContent>
        <TabsContent value="agent">
          <Suspense fallback={<Skeleton className="h-40 w-full rounded-lg" />}>
            <MigrateAgentPanel />
          </Suspense>
        </TabsContent>
        <TabsContent value="takeover">
          <PortTakeoverPanel />
        </TabsContent>
      </Tabs>
    </PageTemplate>
  )
}
