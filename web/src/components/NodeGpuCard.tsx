import {
  CheckCircleIcon,
  CpuIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Progress,
  ProgressLabel,
  ProgressValue,
} from '@/components/ui/progress'
import { formatMiB, vramPercent } from '../lib/models'
import type { NodeGpuResource } from '../types/nodeDetail'

// GPU summary on the node detail page: reserved vs total GPUs (what
// placement counts), VRAM used vs total (what nvidia-smi reports) and
// which apps and models hold the reservations. Renders nothing for a node
// without a GPU.
export function NodeGpuCard({ gpu }: { gpu?: NodeGpuResource }) {
  if (!gpu?.present) return null
  const runtimeMissing = !gpu.runtime_installed
  return (
    <Card>
      <CardHeader className="flex-row items-center justify-between gap-2">
        <CardTitle className="flex items-center gap-2">
          <CpuIcon className="size-4" aria-hidden="true" />
          GPU
        </CardTitle>
        {runtimeMissing ? (
          <Badge variant="warning">
            <WarningIcon className="size-3" aria-hidden="true" />
            nvidia runtime missing
          </Badge>
        ) : (
          <Badge variant="success">
            <CheckCircleIcon className="size-3" aria-hidden="true" />
            ready
          </Badge>
        )}
      </CardHeader>
      <CardContent className="space-y-3">
        <Progress value={vramPercent(gpu.reserved_gpus, gpu.gpu_count)}>
          <ProgressLabel className="text-xs text-muted-foreground">
            GPUs reserved {gpu.reserved_gpus} of {gpu.gpu_count} (
            {gpu.free_gpus} free)
          </ProgressLabel>
          <ProgressValue className="ml-auto text-xs text-muted-foreground" />
        </Progress>
        <Progress value={vramPercent(gpu.used_vram_mib, gpu.total_vram_mib)}>
          <ProgressLabel className="text-xs text-muted-foreground">
            VRAM {formatMiB(gpu.used_vram_mib)} of{' '}
            {formatMiB(gpu.total_vram_mib)}
          </ProgressLabel>
          <ProgressValue className="ml-auto text-xs text-muted-foreground" />
        </Progress>
        {gpu.reservations.length > 0 ? (
          <p className="text-xs text-muted-foreground">
            Reserved by{' '}
            <span className="font-mono text-foreground">
              {gpu.reservations.join(', ')}
            </span>
          </p>
        ) : null}
        {gpu.driver_version ? (
          <p className="text-xs text-muted-foreground">
            Driver {gpu.driver_version}
          </p>
        ) : null}
      </CardContent>
    </Card>
  )
}
