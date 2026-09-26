import { createFileRoute } from '@tanstack/react-router'
import { BellRingingIcon } from '@phosphor-icons/react/dist/ssr'
import { AlertHistoryTable } from '../components/AlertHistoryTable'
import { AlertSilencesPanel } from '../components/AlertSilencesPanel'
import { MaintenanceWindowsPanel } from '../components/MaintenanceWindowsPanel'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

// Platform-wide alert activity: what fired and what happened to each
// notification, plus the silences and maintenance windows that mute
// alerts. Per-app rules stay on /apps/$name/alerts.
export const Route = createFileRoute('/alerts')({
  component: AlertsPage,
})

function AlertsPage() {
  return (
    <div className="space-y-6">
      <div className="flex items-start gap-3">
        <div className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
          <BellRingingIcon className="size-4" aria-hidden="true" />
        </div>
        <div>
          <h1 className="text-lg font-semibold text-foreground">Alerts</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            History of every firing, plus silences and recurring maintenance
            windows. Muted alerts keep evaluating and stay in history.
          </p>
        </div>
      </div>
      <Tabs defaultValue="history">
        <TabsList>
          <TabsTrigger value="history">History</TabsTrigger>
          <TabsTrigger value="silences">Silences</TabsTrigger>
          <TabsTrigger value="maintenance">Maintenance</TabsTrigger>
        </TabsList>
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
