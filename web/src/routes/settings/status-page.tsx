import type { ReactNode } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { RadioTowerGlassIcon } from '@thesvg/ui-react'
import { StatusComponentsPanel } from '../../components/StatusComponentsPanel'
import { StatusIncidentsPanel } from '../../components/StatusIncidentsPanel'
import { StatusPageSettingsPanel } from '../../components/StatusPageSettingsPanel'
import { StatusWebhooksPanel } from '../../components/StatusWebhooksPanel'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { PageHeader } from '@/components/shell/PageHeader'
import { ListSkeleton } from '@/components/ui/list-skeleton'
import {
  useStatusPageTabs,
  type StatusPageTab,
} from '../../lib/settingsPageState'

// Opt-in public status page. Off until switched on; the public page only
// ever shows the public names and statuses chosen here.
export const Route = createFileRoute('/settings/status-page')({
  component: StatusPageSettingsRoute,
})

function StatusPageSettingsRoute() {
  const { t } = useTranslation('statusPage')
  const { loading, tabs } = useStatusPageTabs()
  const has = (tab: StatusPageTab) => tabs.includes(tab)
  let body: ReactNode
  if (loading) {
    body = <ListSkeleton rows={3} />
  } else if (tabs.length === 0) {
    // Off: the settings panel is the single enable card.
    body = <StatusPageSettingsPanel />
  } else {
    body = (
      <Tabs defaultValue="settings">
        <TabsList>
          <TabsTrigger value="settings">{t('page.tabSettings')}</TabsTrigger>
          <TabsTrigger value="components">
            {t('page.tabComponents')}
          </TabsTrigger>
          {has('incidents') ? (
            <TabsTrigger value="incidents">
              {t('page.tabIncidents')}
            </TabsTrigger>
          ) : null}
          {has('webhooks') ? (
            <TabsTrigger value="webhooks">{t('page.tabWebhooks')}</TabsTrigger>
          ) : null}
        </TabsList>
        <TabsContent value="settings" className="pt-3">
          <StatusPageSettingsPanel />
        </TabsContent>
        <TabsContent value="components" className="pt-3">
          <StatusComponentsPanel />
        </TabsContent>
        {has('incidents') ? (
          <TabsContent value="incidents" className="pt-3">
            <StatusIncidentsPanel />
          </TabsContent>
        ) : null}
        {has('webhooks') ? (
          <TabsContent value="webhooks" className="pt-3">
            <StatusWebhooksPanel />
          </TabsContent>
        ) : null}
      </Tabs>
    )
  }
  return (
    <div className="space-y-6">
      <div className="flex items-start gap-3">
        <div className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
          <RadioTowerGlassIcon className="size-5" aria-hidden="true" />
        </div>
        <PageHeader
          title={t('page.title')}
          description={t('page.description')}
        />
      </div>
      {body}
    </div>
  )
}
