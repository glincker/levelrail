import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  BroomGlassIcon,
  ChevronDownGlassIcon,
  TerminalGlassIcon,
} from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsiblePanel,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { RoleGate } from '@/components/ui/RoleGate'
import { Skeleton } from '@/components/ui/skeleton'
import { useAbilities } from '@/hooks/useAbilities'
import { DockerHealthCard } from '../images/DockerHealthCard'
import { SignaturePolicyCard } from '../images/SignaturePolicyCard'
import { PageTemplate } from '../shell/PageTemplate'
import { useImageInventory } from '../../queries/imageInventory'
import { useNodeListOptional } from '../../queries/nodes'
import { useRuntimeSettings } from '../../queries/runtimeSettings'
import { useStorageInventory } from '../../queries/storageInventory'
import { useSystemStatusOptional } from '../../queries/systemStatus'
import { useDiskHistory } from '../../queries/diskHistory'
import { CleanupPlanDrawer } from './CleanupPlanDrawer'
import { parseFilter, type ItemView, type StorageFilter } from './itemRows'
import { StorageCliDialog } from './StorageCliDialog'
import { StorageDiskBar } from './StorageDiskBar'
import { StorageItemList } from './StorageItemList'
import { StoragePolicyStrip } from './StoragePolicyStrip'
import { StorageTrend } from './StorageTrend'
import { StorageVerdict } from './StorageVerdict'
import {
  buildDiskCategories,
  diskHealth,
  safeReclaimableBytes,
} from './storageData'
import { MIN_FORECAST_DAYS } from './trendModel'

interface Props {
  /** Raw `filter` search param; unknown values show everything. */
  filter?: string
  view?: string
  drawerOpen: boolean
  onFilterChange: (filter: StorageFilter | null) => void
  onViewChange: (view: ItemView) => void
  onDrawerChange: (open: boolean) => void
}

function SectionError({
  message,
  onRetry,
}: {
  message: string
  onRetry: () => void
}) {
  const { t } = useTranslation('storage')
  return (
    <Alert>
      <AlertDescription className="flex flex-wrap items-center gap-3">
        {message}
        <Button variant="outline" size="sm" onClick={onRetry}>
          {t('node.retry')}
        </Button>
      </AlertDescription>
    </Alert>
  )
}

export function StorageHubPage({
  filter,
  view,
  drawerOpen,
  onFilterChange,
  onViewChange,
  onDrawerChange,
}: Props) {
  const { t } = useTranslation('storage')
  const [node, setNode] = useState('')
  const { role, can } = useAbilities()
  const nodes = useNodeListOptional().data
  const images = useImageInventory(node)
  const inventory = useStorageInventory(node)
  const status = useSystemStatusOptional()
  const settings = useRuntimeSettings()
  const history = useDiskHistory(30)
  const local = node === ''
  const active = parseFilter(filter)

  const free = status.data?.data_dir_free_bytes
  const total = status.data?.data_dir_total_bytes
  const measured =
    local && typeof free === 'number' && typeof total === 'number'
  const categories = buildDiskCategories({
    usage: status.data?.docker_disk_usage,
    images: images.data,
    inventory: inventory.data,
    local,
  })
  const forecast = history.data?.forecast
  const forecastDays =
    forecast && forecast.basis_days >= MIN_FORECAST_DAYS
      ? forecast.days_until_full
      : null

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

  const actions = (
    <div className="flex flex-wrap items-center gap-2">
      {switcher}
      <StorageCliDialog
        role={role === 'admin' ? 'admin' : 'viewer'}
        node={node || undefined}
      >
        <Button variant="outline" size="sm">
          <TerminalGlassIcon className="size-4" aria-hidden="true" />
          {t('actions.terminal')}
        </Button>
      </StorageCliDialog>
      <RoleGate ability="root">
        <Button
          size="sm"
          onClick={() => {
            onDrawerChange(true)
          }}
        >
          <BroomGlassIcon className="size-4" aria-hidden="true" />
          {t('actions.review')}
        </Button>
      </RoleGate>
    </div>
  )

  let overview
  if (!local) {
    overview = (
      <p className="text-sm text-muted-foreground">{t('node.partial')}</p>
    )
  } else if (status.isPending) {
    overview = <Skeleton className="h-24 w-full" decorative />
  } else if (!measured) {
    overview = (
      <SectionError
        message={t('errors.status')}
        onRetry={() => {
          void status.refetch()
        }}
      />
    )
  } else {
    overview = (
      <div className="flex flex-col gap-3">
        <StorageVerdict
          freeBytes={free}
          totalBytes={total}
          health={diskHealth({
            freeBytes: free,
            totalBytes: total,
            settings: settings.data?.settings,
          })}
          safeReclaimableBytes={safeReclaimableBytes(categories)}
          forecastDays={forecastDays}
        />
        <StorageDiskBar
          totalBytes={total}
          freeBytes={free}
          categories={categories}
          active={active}
          onSelect={(c) => {
            onFilterChange(c === active ? null : c)
          }}
        />
      </div>
    )
  }

  return (
    <PageTemplate
      title={t('title')}
      description={t('subtitle')}
      width="wide"
      actions={actions}
    >
      <div className="flex flex-col gap-4">
        {overview}
        {local ? <StorageTrend /> : null}
        {local ? <StoragePolicyStrip /> : null}
        <StorageItemList
          key={node}
          node={node}
          view={view}
          onViewChange={onViewChange}
          filter={filter}
          onClearFilter={() => {
            onFilterChange(null)
          }}
          buildCacheBytes={
            local
              ? (status.data?.docker_disk_usage?.build_cache_total_bytes ??
                null)
              : null
          }
        />
        {local ? (
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
                canFix={can('root')}
                onFreeSpace={() => {
                  onDrawerChange(true)
                }}
              />
              {images.data ? (
                <SignaturePolicyCard mode={images.data.signatures.mode} />
              ) : null}
            </CollapsiblePanel>
          </Collapsible>
        ) : null}
      </div>
      <CleanupPlanDrawer open={drawerOpen} onOpenChange={onDrawerChange} />
    </PageTemplate>
  )
}
