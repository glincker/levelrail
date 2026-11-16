import { useState } from 'react'
import { createFileRoute, Link } from '@tanstack/react-router'
import { BellRingGlassIcon } from '@thesvg/ui-react'
import { useTranslation } from 'react-i18next'
import { AlertDefaultsPanel } from '../components/AlertDefaultsPanel'
import { AlertInboxPanel } from '../components/AlertInboxPanel'
import { AlertHistoryTable } from '../components/AlertHistoryTable'
import { AlertSilencesPanel } from '../components/AlertSilencesPanel'
import { MaintenanceWindowsPanel } from '../components/MaintenanceWindowsPanel'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { PageHeader } from '@/components/shell/PageHeader'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { useAlertsPageState } from './alertsPageState'

type SidePanel = 'defaults' | 'silences' | 'maintenance'

// Platform-wide alert activity: what fired and what happened to each
// notification, plus the silences and maintenance windows that mute
// alerts. Per-app rules stay on /apps/$name/alerts.
export const Route = createFileRoute('/alerts')({
  component: AlertsPage,
})

function AlertsPage() {
  const { t } = useTranslation('inbox')
  const { state, unread } = useAlertsPageState()
  const [panel, setPanel] = useState<SidePanel | null>(null)
  return (
    <div className="space-y-6">
      <div className="flex items-start gap-3">
        <div className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
          <BellRingGlassIcon className="size-5" aria-hidden="true" />
        </div>
        <PageHeader
          title="Alerts"
          description="History of every firing, plus silences and recurring maintenance windows. Muted alerts keep evaluating and stay in history."
        />
      </div>
      {state === 'empty' && (
        <div className="space-y-4">
          <EmptyState
            icon={<BellRingGlassIcon size="1.4em" />}
            title={t('alertsPage.emptyTitle')}
            description={t('alertsPage.emptyBody')}
            action={
              <Button
                size="sm"
                render={<Link to="/settings/notification-channels" />}
              >
                {t('alertsPage.setup')}
              </Button>
            }
          />
          <nav
            aria-label={t('alertsPage.more')}
            className="flex flex-wrap items-center gap-2"
          >
            {(
              [
                ['defaults', t('defaults.tab')],
                ['silences', t('alertsPage.silences')],
                ['maintenance', t('alertsPage.maintenance')],
              ] as const
            ).map(([id, label]) => (
              <Button
                key={id}
                type="button"
                size="sm"
                variant={panel === id ? 'secondary' : 'outline'}
                aria-pressed={panel === id}
                onClick={() => {
                  setPanel(panel === id ? null : id)
                }}
              >
                {label}
              </Button>
            ))}
          </nav>
          {panel === 'defaults' && <AlertDefaultsPanel />}
          {panel === 'silences' && <AlertSilencesPanel />}
          {panel === 'maintenance' && <MaintenanceWindowsPanel />}
        </div>
      )}
      {(state === 'idle' || state === 'active') && (
        <Tabs defaultValue="inbox">
          <TabsList>
            <TabsTrigger value="inbox">
              {state === 'active'
                ? `${t('tab')} (${t('unreadCount', { count: unread })})`
                : t('tab')}
            </TabsTrigger>
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
      )}
    </div>
  )
}
