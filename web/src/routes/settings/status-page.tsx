import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { RadioTowerGlassIcon } from '@thesvg/ui-react'
import { StatusComponentsPanel } from '../../components/StatusComponentsPanel'
import { StatusIncidentsPanel } from '../../components/StatusIncidentsPanel'
import { StatusPageSettingsPanel } from '../../components/StatusPageSettingsPanel'
import { StatusWebhooksPanel } from '../../components/StatusWebhooksPanel'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { PageHeader } from '@/components/shell/PageHeader'

// Opt-in public status page. Off until switched on; the public page only
// ever shows the public names and statuses chosen here.
export const Route = createFileRoute('/settings/status-page')({
  component: StatusPageSettingsRoute,
})

function StatusPageSettingsRoute() {
  const { t } = useTranslation('statusPage')
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
      <Tabs defaultValue="settings">
        <TabsList>
          <TabsTrigger value="settings">{t('page.tabSettings')}</TabsTrigger>
          <TabsTrigger value="components">
            {t('page.tabComponents')}
          </TabsTrigger>
          <TabsTrigger value="incidents">{t('page.tabIncidents')}</TabsTrigger>
          <TabsTrigger value="webhooks">{t('page.tabWebhooks')}</TabsTrigger>
        </TabsList>
        <TabsContent value="settings" className="pt-3">
          <StatusPageSettingsPanel />
        </TabsContent>
        <TabsContent value="components" className="pt-3">
          <StatusComponentsPanel />
        </TabsContent>
        <TabsContent value="incidents" className="pt-3">
          <StatusIncidentsPanel />
        </TabsContent>
        <TabsContent value="webhooks" className="pt-3">
          <StatusWebhooksPanel />
        </TabsContent>
      </Tabs>
    </div>
  )
}
