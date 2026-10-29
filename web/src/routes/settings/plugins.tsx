import { useState } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { PlugGlassIcon, PlusGlassIcon } from '@thesvg/ui-react'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { TableSkeleton } from '@/components/ui/table-skeleton'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { ApprovalDialog } from '../../components/plugins/ApprovalDialog'
import { InstallPluginDialog } from '../../components/plugins/InstallPluginDialog'
import { PanelHost } from '../../components/plugins/PanelHost'
import { MarketplaceTab } from '../../components/plugins/MarketplaceTab'
import { PluginPanels } from '../../components/plugins/PluginPanels'
import { PluginTable } from '../../components/plugins/PluginTable'
import { TrustedKeysCard } from '../../components/plugins/TrustedKeysCard'
import { requireExperimental } from '../../lib/experimental'
import {
  pluginKeysQueryOptions,
  pluginsQueryOptions,
  usePlugins,
  type Plugin,
} from '../../queries/plugins'

export const Route = createFileRoute('/settings/plugins')({
  beforeLoad: ({ context: { queryClient }, location }) =>
    requireExperimental(queryClient, 'plugins', location.href),
  loader: ({ context: { queryClient } }) =>
    Promise.all([
      queryClient.ensureQueryData(pluginsQueryOptions()),
      queryClient.ensureQueryData(pluginKeysQueryOptions()),
    ]),
  component: PluginsPage,
  pendingComponent: () => <TableSkeleton columnCount={5} rowCount={3} />,
})

function PluginsPage() {
  const { t } = useTranslation('plugins')
  const { data: plugins = [], isLoading, error } = usePlugins()
  const [installing, setInstalling] = useState(false)
  const [reviewing, setReviewing] = useState<Plugin | null>(null)
  const [upgrading, setUpgrading] = useState<Plugin | null>(null)
  const showKeys = () =>
    document
      .getElementById('plugin-trusted-keys')
      ?.scrollIntoView({ behavior: 'smooth' })

  return (
    <PageTemplate
      icon={<PlugGlassIcon size="1.2em" />}
      title={t('title')}
      description={t('description')}
      helpPath="/plugins"
      actions={
        <Button type="button" onClick={() => setInstalling(true)}>
          <PlusGlassIcon size="1.2em" aria-hidden="true" />
          {t('install')}
        </Button>
      }
    >
      <Tabs defaultValue="installed">
        <TabsList>
          <TabsTrigger value="installed">{t('tabs.installed')}</TabsTrigger>
          <TabsTrigger value="marketplace">{t('tabs.marketplace')}</TabsTrigger>
        </TabsList>
        <TabsContent value="installed" className="space-y-4">
          {error ? (
            <Alert variant="destructive">
              <AlertTitle>{t('loadFailed')}</AlertTitle>
              <AlertDescription>
                {error.message} {t('loadFailedHint')}
              </AlertDescription>
            </Alert>
          ) : isLoading ? (
            <TableSkeleton columnCount={5} rowCount={3} />
          ) : plugins.length === 0 ? (
            <EmptyState
              className="gap-2 border-0 bg-transparent py-4"
              icon={<PlugGlassIcon className="size-5.5" />}
              title={t('empty.title')}
              description={t('empty.description')}
              action={
                <Button type="button" onClick={() => setInstalling(true)}>
                  {t('empty.action')}
                </Button>
              }
            />
          ) : (
            <PluginTable
              plugins={plugins}
              onReview={setReviewing}
              onUpgrade={setUpgrading}
              onTrustKeys={showKeys}
            />
          )}
          <PluginPanels plugins={plugins} />
          <PanelHost slot="plugins" />
          <div id="plugin-trusted-keys">
            <TrustedKeysCard />
          </div>
        </TabsContent>
        <TabsContent value="marketplace">
          <MarketplaceTab />
        </TabsContent>
      </Tabs>
      <InstallPluginDialog
        open={installing}
        onClose={() => setInstalling(false)}
        onInstalled={(plugin) => {
          setInstalling(false)
          if (plugin.approved_at === '') setReviewing(plugin)
        }}
      />
      <InstallPluginDialog
        key={upgrading?.name ?? 'none'}
        open={upgrading !== null}
        upgradeOf={upgrading?.name}
        onClose={() => setUpgrading(null)}
        onInstalled={(plugin) => {
          setUpgrading(null)
          if (plugin.approved_at === '') setReviewing(plugin)
        }}
      />
      <ApprovalDialog plugin={reviewing} onClose={() => setReviewing(null)} />
    </PageTemplate>
  )
}
