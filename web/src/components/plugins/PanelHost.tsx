import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { toast } from '@/components/ui/toast'
import {
  usePanelAction,
  usePanelData,
  usePluginPanels,
  type PanelSlot,
  type PluginPanelEntry,
} from '../../queries/pluginPanels'
import { PanelWidget } from './PanelWidgets'

function PanelCard({
  entry,
  appId,
}: Readonly<{ entry: PluginPanelEntry; appId?: string }>) {
  const { t } = useTranslation('plugins')
  const { plugin, publisher, panel } = entry
  const data = usePanelData(plugin, panel.id, appId)
  const action = usePanelAction(plugin, panel.id)

  const press = async (name: string) => {
    try {
      const res = await action.mutateAsync({ action: name, appId })
      toast.add({
        type: res.ok ? 'success' : 'error',
        title: res.ok ? t('panelHost.actionDone') : t('panelHost.actionFailed'),
        description: res.message || undefined,
      })
      void data.refetch()
    } catch (error) {
      toast.add({
        type: 'error',
        title: t('panelHost.actionFailed'),
        description: error instanceof Error ? error.message : undefined,
      })
    }
  }

  return (
    <article className="rounded-lg border bg-card p-4 text-card-foreground">
      <h3 className="font-medium">{panel.title}</h3>
      <p className="text-xs text-muted-foreground">
        {t('panels.from', { name: plugin, publisher })}
      </p>
      {panel.description ? (
        <p className="mt-2 text-sm">{panel.description}</p>
      ) : null}
      <div className="mt-3 space-y-4">
        {data.isPending ? (
          <div aria-busy="true" className="space-y-2">
            <Skeleton className="h-6 w-1/3" />
            <Skeleton className="h-16 w-full" />
          </div>
        ) : null}
        {data.isError ? (
          <div className="space-y-2" role="alert">
            <p className="text-sm text-muted-foreground">
              {t('panelHost.loadFailed')}
            </p>
            <p className="text-xs break-words text-muted-foreground">
              {data.error.message}
            </p>
            <Button
              variant="outline"
              size="sm"
              disabled={data.isFetching}
              onClick={() => void data.refetch()}
            >
              {t('panelHost.retry')}
            </Button>
          </div>
        ) : null}
        {data.data?.widgets.map((w, i) => (
          <PanelWidget
            key={`${w.type}-${i}`}
            w={w}
            onPress={press}
            pending={action.isPending}
          />
        ))}
      </div>
    </article>
  )
}

/**
 * Renders every enabled plugin panel placed in `slot`. A failing panel shows its
 * own error and Retry; the host renders nothing when no panel targets the slot.
 */
export function PanelHost({
  slot,
  appId,
}: Readonly<{ slot: PanelSlot; appId?: string }>) {
  const { t } = useTranslation('plugins')
  const { data } = usePluginPanels(slot)
  if (!data || data.length === 0) return null
  return (
    <section className="space-y-3" aria-label={t('panels.title')}>
      <h2 className="text-base font-medium">{t('panels.title')}</h2>
      <div className="grid gap-3 md:grid-cols-2">
        {data.map((entry) => (
          <PanelCard
            key={`${entry.plugin}/${entry.panel.id}`}
            entry={entry}
            appId={appId}
          />
        ))}
      </div>
    </section>
  )
}
