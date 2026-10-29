import { createFileRoute } from '@tanstack/react-router'
import { BellRingGlassIcon } from '@thesvg/ui-react'
import { useTranslation } from 'react-i18next'
import { AlertDefaultsPanel } from '../components/AlertDefaultsPanel'
import { AlertInboxPanel } from '../components/AlertInboxPanel'
import { AlertHistoryTable } from '../components/AlertHistoryTable'
import { AlertSilencesPanel } from '../components/AlertSilencesPanel'
import { MaintenanceWindowsPanel } from '../components/MaintenanceWindowsPanel'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { PageHeader } from '@/components/shell/PageHeader'

// Platform-wide alert activity: what fired and what happened to each
// notification, plus the silences and maintenance windows that mute
// alerts. Per-app rules stay on /apps/$name/alerts.
export const Route = createFileRoute('/alerts')({
  component: AlertsPage,
})

function AlertsPage() {
  const { t } = useTranslation('inbox')
  return (
    <div className="space-y-6">
      <div className="flex items-start gap-3">
        <div className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
          <BellRingGlassIcon className="size-4.5" aria-hidden="true" />
        </div>
        <PageHeader
          title="Alerts"
          description="History of every firing, plus silences and recurring maintenance windows. Muted alerts keep evaluating and stay in history."
        />
      </div>
      <Tabs defaultValue="inbox">
        <TabsList>
          <TabsTrigger value="inbox">{t('tab')}</TabsTrigger>
          <TabsTrigger value="history">History</TabsTrigger>
          <TabsTrigger value="silences">Silences</TabsTrigger>
          <TabsTrigger value="maintenance">Maintenance</TabsTrigger>
          <TabsTrigger value="defaults">{t('defaults.tab')}</TabsTrigger>
        </TabsList>
        <TabsContent value="defaults" className="pt-3">
          <AlertDefaultsPanel />
        </TabsContent>
        <TabsContent value="inbox" className="pt-3">
          <AlertInboxPanel />
        </TabsContent>
        <TabsContent value="history" className="pt-3">
          <AlertHistoryTable />
        </TabsContent>
        <TabsContent value="silences" className="pt-3">
          <AlertSilencesPanel />
        </TabsContent>
        <TabsContent value="maintenance" className="pt-3">
          <MaintenanceWindowsPanel />
        </TabsContent>
      </Tabs>
    </div>
  )
}
