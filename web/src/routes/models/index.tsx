import { requireExperimental } from '../../lib/experimental'
import { createFileRoute } from '@tanstack/react-router'
import { useVirtualizer } from '@tanstack/react-virtual'
import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { BotGlassIcon, CpuGlassIcon } from '@thesvg/ui-react'
import { DeployModelDialog } from '../../components/DeployModelDialog'
import { AddEndpointDialog } from '../../components/models/AddEndpointDialog'
import { GpuNodeCard } from '../../components/GpuNodeCard'
import { ModelCacheCard } from '../../components/ModelCacheCard'
import { ModelKeyRevealDialog } from '../../components/ModelKeyRevealDialog'
import {
  MODEL_LIST_GRID,
  ModelRow,
  ModelRowSkeleton,
} from '../../components/ModelRow'
import { GhostPreview } from '../../components/GhostPreview'
import { EmptyState } from '../../components/ui/empty-state'
import { PageHeader } from '../../components/shell/PageHeader'
import { useGpuNodes, useModels } from '../../queries/models'
import type { CreateModelResponse } from '../../types/models'

export const Route = createFileRoute('/models/')({
  beforeLoad: ({ context: { queryClient }, location }) =>
    requireExperimental(queryClient, 'ai-models', location.href),
  component: ModelsPage,
})

const ROW_HEIGHT = 64

function SampleRows() {
  const { t } = useTranslation('models')
  return (
    <div>
      <ListHeader />
      <div
        className={`${MODEL_LIST_GRID} items-center border-b border-border px-4 py-3 text-sm`}
      >
        <span className="font-medium text-foreground">
          {t('empty.sample.modelName')}
        </span>
        <span>{t('empty.sample.engine')}</span>
        <span>{t('empty.sample.placement')}</span>
        <span className="text-foreground">{t('empty.sample.status')}</span>
        <span className="text-muted-foreground">{t('empty.sample.vram')}</span>
        <span aria-hidden="true" className="w-24" />
      </div>
    </div>
  )
}

function ListHeader() {
  return (
    <div
      className={`${MODEL_LIST_GRID} sticky top-0 z-10 border-b border-border bg-card px-4 py-2 text-xs font-medium tracking-wide text-muted-foreground uppercase`}
    >
      <span>Model</span>
      <span>Engine</span>
      <span>Placement</span>
      <span>Status</span>
      <span>Detail</span>
      <span aria-hidden="true" className="w-24" />
    </div>
  )
}

function GpuSection() {
  const { t } = useTranslation('models')
  const gpus = useGpuNodes()
  const nodes = (gpus.data ?? []).filter((n) => n.present)
  if (nodes.length === 0) {
    return (
      <EmptyState
        icon={<CpuGlassIcon className="size-5.5" />}
        title={t('empty.gpus.title')}
        description={t('empty.gpus.description')}
        helpPath="/ai-models#gpu-nodes"
        className="py-8"
      />
    )
  }
  return (
    <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
      {nodes.map((node) => (
        <GpuNodeCard key={node.node_id || node.name} node={node} />
      ))}
    </div>
  )
}

function ModelList({
  onDeployed,
}: {
  onDeployed: (created: CreateModelResponse) => void
}) {
  const { t } = useTranslation('models')
  const { data, isPending, isError, error } = useModels()
  const parentRef = useRef<HTMLDivElement>(null)
  const models = data ?? []

  const virtualizer = useVirtualizer({
    count: models.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => ROW_HEIGHT,
    overscan: 8,
  })

  if (isPending) {
    return (
      <div className="overflow-hidden rounded-lg border border-border bg-card">
        <ListHeader />
        {Array.from({ length: 3 }, (_, i) => (
          <ModelRowSkeleton key={i} />
        ))}
      </div>
    )
  }
  if (isError) {
    return (
      <p role="alert" className="text-sm text-destructive">
        {error.message}
      </p>
    )
  }
  if (models.length === 0) {
    return (
      <GhostPreview sample={<SampleRows />} className="min-h-72">
        <EmptyState
          icon={<BotGlassIcon className="size-5.5" />}
          title={t('empty.models.title')}
          description={t('empty.models.description')}
          action={<DeployModelDialog onDeployed={onDeployed} />}
          helpPath="/ai-models"
        />
      </GhostPreview>
    )
  }
  return (
    <div
      ref={parentRef}
      className="max-h-[60vh] overflow-auto rounded-lg border border-border bg-card"
    >
      <ListHeader />
      <div style={{ height: virtualizer.getTotalSize(), position: 'relative' }}>
        {virtualizer.getVirtualItems().map((row) => {
          const model = models[row.index]
          if (!model) return null
          return (
            <div
              key={row.key}
              data-index={row.index}
              ref={virtualizer.measureElement}
              style={{
                position: 'absolute',
                top: 0,
                left: 0,
                width: '100%',
                transform: `translateY(${row.start}px)`,
              }}
            >
              <ModelRow model={model} />
            </div>
          )
        })}
      </div>
    </div>
  )
}

function ModelsPage() {
  const [created, setCreated] = useState<CreateModelResponse | null>(null)
  return (
    <div className="space-y-6">
      <PageHeader
        title="AI models"
        actions={
          <div className="flex items-center gap-2">
            <AddEndpointDialog onAdded={setCreated} />
            <DeployModelDialog onDeployed={setCreated} />
          </div>
        }
      />
      <section aria-labelledby="gpu-nodes-heading" className="space-y-3">
        <h2
          id="gpu-nodes-heading"
          className="text-sm font-medium text-foreground"
        >
          GPU nodes
        </h2>
        <GpuSection />
      </section>
      <section aria-labelledby="models-heading" className="space-y-3">
        <h2 id="models-heading" className="text-sm font-medium text-foreground">
          Models
        </h2>
        <ModelList onDeployed={setCreated} />
      </section>
      <section aria-labelledby="model-cache-heading" className="space-y-3">
        <h2
          id="model-cache-heading"
          className="text-sm font-medium text-foreground"
        >
          Model cache
        </h2>
        <ModelCacheCard />
      </section>
      {created ? (
        <ModelKeyRevealDialog
          modelName={created.name}
          apiKey={created.api_key}
          baseUrl={created.endpoint_url}
          onClose={() => {
            setCreated(null)
          }}
        />
      ) : null}
    </div>
  )
}
