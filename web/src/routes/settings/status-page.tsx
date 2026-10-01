import { createFileRoute } from '@tanstack/react-router'
import { BroadcastIcon } from '@phosphor-icons/react/dist/ssr'
import { StatusComponentsPanel } from '../../components/StatusComponentsPanel'
import { StatusIncidentsPanel } from '../../components/StatusIncidentsPanel'
import { StatusPageSettingsPanel } from '../../components/StatusPageSettingsPanel'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { PageHeader } from '@/components/shell/PageHeader'

// Opt-in public status page. Off until switched on; the public page only
// ever shows the public names and statuses chosen here.
export const Route = createFileRoute('/settings/status-page')({
  component: StatusPageSettingsRoute,
})

function StatusPageSettingsRoute() {
  return (
    <div className="space-y-6">
      <div className="flex items-start gap-3">
        <div className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
          <BroadcastIcon className="size-4" aria-hidden="true" />
        </div>
        <PageHeader
          title="Status page"
          description="A read-only public page with component status, 90 day uptime bars, incidents and maintenance announcements. It exposes nothing but the public names and statuses you choose."
        />
      </div>
      <Tabs defaultValue="settings">
        <TabsList>
          <TabsTrigger value="settings">Settings</TabsTrigger>
          <TabsTrigger value="components">Components</TabsTrigger>
          <TabsTrigger value="incidents">Incidents and maintenance</TabsTrigger>
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
      </Tabs>
    </div>
  )
}
