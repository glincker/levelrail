import {
  CpuIcon,
  WarningIcon,
  CheckCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import {
  Progress,
  ProgressLabel,
  ProgressValue,
} from '@/components/ui/progress'
import { formatMiB, vramPercent } from '../lib/models'
import type { GpuNode } from '../types/models'

// One GPU node: driver, per-GPU VRAM used vs total, and how many models
// run there. A node with a GPU but no nvidia container runtime shows the
// install hint, since models cannot start there until it is fixed.
export function GpuNodeCard({ node }: { node: GpuNode }) {
  const runtimeMissing = node.present && !node.runtime_installed
  return (
    <section
      className="rounded-lg border border-border bg-card p-4"
      aria-label={`GPU node ${node.name}`}
    >
      <header className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <h3 className="flex items-center gap-2 text-sm font-semibold text-foreground">
            <CpuIcon className="size-4 shrink-0" aria-hidden="true" />
            <span className="truncate">{node.name}</span>
            {node.is_local ? <Badge variant="outline">this host</Badge> : null}
          </h3>
          <p className="mt-0.5 text-xs text-muted-foreground">
            {node.gpu_count} GPU{node.gpu_count === 1 ? '' : 's'}
            {node.driver_version ? `, driver ${node.driver_version}` : ''}
            {`, ${node.model_count} model${node.model_count === 1 ? '' : 's'}`}
          </p>
        </div>
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
      </header>

      <Progress
        value={vramPercent(node.used_vram_mib, node.total_vram_mib)}
        className="mt-3"
      >
        <ProgressLabel className="text-xs text-muted-foreground">
          VRAM {formatMiB(node.used_vram_mib)} of{' '}
          {formatMiB(node.total_vram_mib)}
        </ProgressLabel>
        <ProgressValue className="ml-auto text-xs text-muted-foreground" />
      </Progress>

      <ul className="mt-3 space-y-1.5">
        {node.devices.map((d) => (
          <li
            key={d.uuid || d.index}
            className="flex items-center justify-between gap-2 text-xs"
          >
            <span className="min-w-0 truncate text-foreground">
              GPU {d.index}: {d.name}
            </span>
            <span className="shrink-0 font-mono text-muted-foreground">
              {formatMiB(d.vram_used_mib)} / {formatMiB(d.vram_total_mib)},{' '}
              {d.utilization_percent}%
            </span>
          </li>
        ))}
      </ul>

      {runtimeMissing && node.hint ? (
        <p className="mt-3 rounded-md bg-muted p-2 text-xs text-muted-foreground">
          {node.hint}
        </p>
      ) : null}
    </section>
  )
}
