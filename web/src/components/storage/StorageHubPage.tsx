import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronDownGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsiblePanel,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs'
import { BuildCachePanel } from '../builds/BuildCachePanel'
import { DockerHealthCard } from '../images/DockerHealthCard'
import { ImagesSection } from '../images/ImagesPage'
import type { ImageFilter } from '../images/imageFilters'
import { SignaturePolicyCard } from '../images/SignaturePolicyCard'
import { PageTemplate } from '../shell/PageTemplate'
import { useIsRoot } from '../../hooks/useIsRoot'
import { useImageInventory } from '../../queries/imageInventory'
import { useNodeListOptional } from '../../queries/nodes'
import { useSystemStatusOptional } from '../../queries/systemStatus'
import { StorageBreakdownBar } from './StorageBreakdownBar'
import { buildSegments } from './storageBreakdown'
import { parseStorageTab, type StorageTab } from './storageTabs'

interface Props {
  tab: StorageTab
  filter: ImageFilter
  /** Opens the image cleanup preview on arrival, for links from disk banners. */
  review?: boolean
  onTabChange: (tab: StorageTab) => void
  onFilterChange: (filter: ImageFilter) => void
}

export function StorageHubPage({
  tab,
  filter,
  review = false,
  onTabChange,
  onFilterChange,
}: Props) {
  const { t } = useTranslation('storage')
  const [node, setNode] = useState('')
  const [gcOpen, setGcOpen] = useState(review)
  const [pruneOpen, setPruneOpen] = useState(false)
  const isRoot = useIsRoot()
  const nodes = useNodeListOptional().data
  const inventory = useImageInventory(node)
  const status = useSystemStatusOptional()
  const local = node === ''
  const totals = inventory.data?.totals
  const segments = buildSegments({
    usage: status.data?.docker_disk_usage,
    images: totals
      ? {
          sizeBytes: totals.size_bytes,
          reclaimableBytes:
            totals.reclaimable_bytes + totals.unmanaged_unused_bytes,
        }
      : undefined,
    local,
  })
  // A remote node whose inventory fails is shown as unsupported, not as a page error.
  const unsupported = !local && inventory.isError

  const switcher =
    nodes && nodes.length > 0 ? (
      <label className="flex items-center gap-2 text-sm text-muted-foreground">
        {t('node.label')}
        <select
          className="h-8 rounded-md border border-input bg-background px-2 text-sm text-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
          value={node}
          onChange={(e) => {
            setNode(e.target.value)
          }}
        >
          <option value="">{t('node.local')}</option>
          {nodes.map((n) => (
            <option key={n.id} value={n.id}>
              {n.name}
            </option>
          ))}
        </select>
      </label>
    ) : null

  return (
    <PageTemplate
      title={t('title')}
      description={t('subtitle')}
      width="wide"
      actions={switcher}
    >
      <div className="flex flex-col gap-4">
        <StorageBreakdownBar
          segments={segments}
          loading={status.isPending || (inventory.isPending && local)}
          freeBytes={local ? status.data?.data_dir_free_bytes : undefined}
          totalBytes={local ? status.data?.data_dir_total_bytes : undefined}
        />
        {!local && !unsupported ? (
          <p className="text-sm text-muted-foreground">{t('node.partial')}</p>
        ) : null}
        <Tabs
          value={tab}
          onValueChange={(next) => {
            onTabChange(parseStorageTab(next))
          }}
        >
          <TabsList variant="line" aria-label={t('tabs.label')}>
            <TabsTrigger value="images">{t('tabs.images')}</TabsTrigger>
            <TabsTrigger value="cache">{t('tabs.cache')}</TabsTrigger>
          </TabsList>
          <TabsContent value="images" className="mt-2 flex flex-col gap-4">
            {unsupported ? (
              <Alert>
                <AlertTitle>{t('node.unsupportedTitle')}</AlertTitle>
                <AlertDescription className="flex flex-wrap items-center gap-3">
                  {t('node.unsupported')}
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => {
                      void inventory.refetch()
                    }}
                  >
                    {t('node.retry')}
                  </Button>
                </AlertDescription>
              </Alert>
            ) : (
              <ImagesSection
                key={node}
                node={node}
                gcOpen={gcOpen}
                onGcOpenChange={setGcOpen}
                filter={filter}
                onFilterChange={onFilterChange}
              />
            )}
          </TabsContent>
          <TabsContent value="cache" className="mt-2">
            <BuildCachePanel
              pruneOpen={pruneOpen}
              onPruneOpenChange={setPruneOpen}
            />
          </TabsContent>
        </Tabs>
        <Collapsible className="rounded-lg border border-border px-4 py-3">
          <CollapsibleTrigger className="group flex items-center justify-between gap-2 text-sm font-medium focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none">
            {t('engine.title')}
            <ChevronDownGlassIcon
              aria-hidden="true"
              className="size-5 text-muted-foreground transition-transform group-data-[panel-open]:rotate-180 motion-reduce:transition-none"
            />
          </CollapsibleTrigger>
          <CollapsiblePanel className="mt-3 flex flex-col gap-4">
            <DockerHealthCard
              canFix={isRoot}
              onFreeSpace={() => {
                onTabChange('images')
                setGcOpen(true)
              }}
            />
            {inventory.data ? (
              <SignaturePolicyCard mode={inventory.data.signatures.mode} />
            ) : null}
          </CollapsiblePanel>
        </Collapsible>
      </div>
    </PageTemplate>
  )
}
