import { useState } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { PlugsConnectedIcon, PlusIcon } from '@phosphor-icons/react/dist/ssr'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { TableSkeleton } from '@/components/ui/table-skeleton'
import { ApprovalDialog } from '../../components/plugins/ApprovalDialog'
import { InstallPluginDialog } from '../../components/plugins/InstallPluginDialog'
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
      icon={<PlugsConnectedIcon />}
      title={t('title')}
      description={t('description')}
      helpPath="/plugins"
      actions={
        <Button type="button" onClick={() => setInstalling(true)}>
          <PlusIcon aria-hidden="true" />
          {t('install')}
        </Button>
      }
    >
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
          icon={<PlugsConnectedIcon className="size-5" />}
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
      <div id="plugin-trusted-keys">
        <TrustedKeysCard />
      </div>
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
