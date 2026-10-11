import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Button } from '@/components/ui/button'
import { PageHeader } from '@/components/shell/PageHeader'
import { SettingsCardSkeleton } from '@/components/settings/SettingsSkeletons'
import { postureQueryOptions } from '../../queries/securityCenter'
import {
  PostureChecklist,
  PostureScoreCard,
} from '../../components/security/PostureOverview'
import { SessionsPanel } from '../../components/security/SessionsPanel'
import { PolicyPanel } from '../../components/security/PolicyPanel'

const TABS = ['checklist', 'sessions', 'policy'] as const
type SecurityTab = (typeof TABS)[number]

interface SecuritySearch {
  tab?: SecurityTab
}

// Plain function, not zod, so validateSearch stays out of the eager bundle.
function validateSecuritySearch(
  search: Record<string, unknown>,
): SecuritySearch {
  const tab = TABS.find((x) => x === search.tab)
  return tab && tab !== 'checklist' ? { tab } : {}
}

export const Route = createFileRoute('/security/')({
  validateSearch: validateSecuritySearch,
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(postureQueryOptions()),
  component: SecurityCenterPage,
  pendingComponent: () => <SettingsCardSkeleton rows={4} rowVariant="line" />,
})

function SecurityCenterPage() {
  const { t } = useTranslation('security')
  const { tab = 'checklist' } = Route.useSearch()
  const navigate = useNavigate({ from: Route.fullPath })
  const posture = useQuery(postureQueryOptions())
  const setTab = (next: SecurityTab) => {
    void navigate({ search: next === 'checklist' ? {} : { tab: next } })
  }
  const toSessions = () => setTab('sessions')
  return (
    <div className="space-y-6">
      <PageHeader
        title={t('page.title')}
        description={t('page.description')}
        helpPath="/security-center"
      />
      <Tabs
        value={tab}
        onValueChange={(v) => setTab(TABS.find((x) => x === v) ?? 'checklist')}
      >
        <TabsList>
          <TabsTrigger value="checklist">
            {t('page.tabs.checklist')}
          </TabsTrigger>
          <TabsTrigger value="sessions">{t('page.tabs.sessions')}</TabsTrigger>
          <TabsTrigger value="policy">{t('page.tabs.policy')}</TabsTrigger>
        </TabsList>
        <TabsContent value="checklist" className="space-y-6 pt-4">
          {posture.error ? (
            <div className="space-y-2 text-sm">
              <p className="text-destructive">{t('page.loadError')}</p>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => void posture.refetch()}
              >
                {t('page.retry')}
              </Button>
            </div>
          ) : null}
          {posture.data ? (
            <>
              <PostureScoreCard posture={posture.data} />
              {posture.data.account.length > 0 ? (
                <PostureChecklist
                  title={t('checklist.account')}
                  items={posture.data.account}
                  onReviewSessions={toSessions}
                />
              ) : null}
              {posture.data.full ? (
                <PostureChecklist
                  title={t('checklist.platform')}
                  items={posture.data.items}
                  onReviewSessions={toSessions}
                />
              ) : null}
            </>
          ) : null}
        </TabsContent>
        <TabsContent value="sessions" className="pt-4">
          <SessionsPanel />
        </TabsContent>
        <TabsContent value="policy" className="pt-4">
          <PolicyPanel />
        </TabsContent>
      </Tabs>
    </div>
  )
}
