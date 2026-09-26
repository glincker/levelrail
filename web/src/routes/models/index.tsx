import { createFileRoute } from '@tanstack/react-router'
import { useVirtualizer } from '@tanstack/react-virtual'
import { useRef, useState } from 'react'
import { CpuIcon, RobotIcon } from '@phosphor-icons/react/dist/ssr'
import { DeployModelDialog } from '../../components/DeployModelDialog'
import { GpuNodeCard } from '../../components/GpuNodeCard'
import { ModelCacheCard } from '../../components/ModelCacheCard'
import { ModelKeyRevealDialog } from '../../components/ModelKeyRevealDialog'
import {
  MODEL_LIST_GRID,
  ModelRow,
  ModelRowSkeleton,
} from '../../components/ModelRow'
import { EmptyState } from '../../components/ui/empty-state'
import { useGpuNodes, useModels } from '../../queries/models'
import type { CreateModelResponse } from '../../types/models'

export const Route = createFileRoute('/models/')({
  component: ModelsPage,
})

const ROW_HEIGHT = 64

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
  const gpus = useGpuNodes()
  const nodes = (gpus.data ?? []).filter((n) => n.present)
  if (nodes.length === 0) {
    return (
      <EmptyState
        icon={<CpuIcon className="size-5" />}
        title="No GPU node reported yet"
        description="Nodes with an NVIDIA GPU and driver appear here with their VRAM. Install the NVIDIA driver and the nvidia-container-toolkit on an Ubuntu node, then wait a minute for it to report."
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

function ModelList() {
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
      <EmptyState
        icon={<RobotIcon className="size-5" />}
        title="No models deployed"
        description="Deploy Ollama, vLLM or llama.cpp on a GPU node and get an OpenAI-compatible endpoint with an API key."
      />
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
      <div className="flex items-baseline justify-between gap-3">
        <h1 className="text-lg font-semibold text-foreground">AI models</h1>
        <DeployModelDialog onDeployed={setCreated} />
      </div>
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
        <ModelList />
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
