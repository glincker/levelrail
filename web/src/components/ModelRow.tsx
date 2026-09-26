import {
  ArrowClockwiseIcon,
  CheckCircleIcon,
  CircleNotchIcon,
  KeyIcon,
  WarningCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { StatusBadge } from '@/components/ui/status-badge'
import { toast } from '@/components/ui/toast'
import { gpuSummary, modelPhase, nodeLabel } from '../lib/models'
import { useRestartModel, useRotateModelApiKey } from '../queries/models'
import type { ModelResource } from '../types/models'
import { DeleteModelDialog } from './DeleteModelDialog'
import { ModelKeyRevealDialog } from './ModelKeyRevealDialog'
import { ModelKeysDialog } from './ModelKeysDialog'
import { useState } from 'react'

// Shared column grid between the sticky header (routes/models/index.tsx)
// and every row, so header labels line up with row content.
export const MODEL_LIST_GRID =
  'grid grid-cols-[minmax(0,1.2fr)_minmax(0,0.7fr)_minmax(0,1.6fr)_minmax(0,1fr)_minmax(0,2fr)_auto] items-center gap-3'

function ModelStatusBadge({ model }: { model: ModelResource }) {
  switch (modelPhase(model)) {
    case 'ready':
      return (
        <StatusBadge variant="success" label="Loaded" icon={CheckCircleIcon} />
      )
    case 'progress':
      return (
        <StatusBadge
          variant="warning"
          label={model.status.reason}
          icon={CircleNotchIcon}
        />
      )
    case 'deleting':
      return (
        <StatusBadge variant="muted" label="Deleting" icon={CircleNotchIcon} />
      )
    default:
      return (
        <StatusBadge
          variant="destructive"
          label={model.status.reason}
          icon={WarningCircleIcon}
        />
      )
  }
}

export function ModelRow({ model }: { model: ModelResource }) {
  const restart = useRestartModel()
  const rotate = useRotateModelApiKey()
  const [newKey, setNewKey] = useState<string | null>(null)
  const deleting = model.status.reason === 'Deleting'

  return (
    <div
      className={`${MODEL_LIST_GRID} h-full w-full border-b border-border px-4 py-3`}
    >
      <span className="min-w-0">
        <span className="block truncate text-sm font-medium text-foreground">
          {model.name}
        </span>
        <span className="block truncate font-mono text-xs text-muted-foreground">
          {model.model}
        </span>
      </span>
      <span className="min-w-0">
        <Badge variant="outline" className="font-mono text-[11px]">
          {model.engine}
        </Badge>
      </span>
      <span className="min-w-0 text-xs text-muted-foreground">
        <span className="block truncate">
          {nodeLabel(model.node_id)}, {gpuSummary(model)}
        </span>
        {model.endpoint_url ? (
          <span className="block truncate font-mono">{model.endpoint_url}</span>
        ) : null}
      </span>
      <span className="min-w-0">
        <ModelStatusBadge model={model} />
      </span>
      <span
        className="min-w-0 truncate text-xs text-muted-foreground"
        title={model.status.message}
      >
        {model.status.message}
      </span>
      <span className="flex items-center gap-1 justify-self-end">
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label={`Restart ${model.name}`}
          title="Restart"
          disabled={deleting || restart.isPending}
          onClick={() => {
            restart.mutate(model.name, {
              onSuccess: () => {
                toast.add({
                  title: `Restarting ${model.name}.`,
                  type: 'success',
                })
              },
              onError: (error) => {
                toast.add({
                  title: 'Could not restart model.',
                  description: error.message,
                  type: 'error',
                })
              },
            })
          }}
        >
          <ArrowClockwiseIcon aria-hidden="true" />
        </Button>
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label={`Rotate API key of ${model.name}`}
          title="Rotate API key"
          disabled={deleting || rotate.isPending}
          onClick={() => {
            rotate.mutate(model.name, {
              onSuccess: (res) => {
                setNewKey(res.api_key)
              },
              onError: (error) => {
                toast.add({
                  title: 'Could not rotate the API key.',
                  description: error.message,
                  type: 'error',
                })
              },
            })
          }}
        >
          <KeyIcon aria-hidden="true" />
        </Button>
        <ModelKeysDialog
          name={model.name}
          baseUrl={model.endpoint_url}
          disabled={deleting}
        />
        <DeleteModelDialog name={model.name} disabled={deleting} />
      </span>
      {newKey ? (
        <ModelKeyRevealDialog
          modelName={model.name}
          apiKey={newKey}
          baseUrl={model.endpoint_url}
          onClose={() => {
            setNewKey(null)
          }}
        />
      ) : null}
    </div>
  )
}

export function ModelRowSkeleton() {
  return (
    <div
      className={`${MODEL_LIST_GRID} border-b border-border px-4 py-3`}
      aria-hidden="true"
    >
      <div className="h-4 w-32 animate-pulse rounded bg-muted" />
      <div className="h-4 w-14 animate-pulse rounded bg-muted" />
      <div className="h-4 w-40 animate-pulse rounded bg-muted" />
      <div className="h-5 w-20 animate-pulse rounded bg-muted" />
      <div className="h-4 w-48 animate-pulse rounded bg-muted" />
      <div className="h-6 w-20 animate-pulse rounded bg-muted" />
    </div>
  )
}
