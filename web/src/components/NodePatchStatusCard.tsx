import {
  CheckCircleIcon,
  QuestionIcon,
  WarningCircleIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import type { Icon } from '@phosphor-icons/react'
import { useNodePatchStatus } from '../queries/nodes'
import type { NodePatchStatusResource } from '../types/nodeDetail'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge, type badgeVariants } from '@/components/ui/badge'
import type { VariantProps } from 'class-variance-authority'

type PatchState = 'loading' | 'unknown' | 'up-to-date' | 'updates' | 'security'

type PatchStatusConfig = {
  icon: Icon
  variant: VariantProps<typeof badgeVariants>['variant']
  label: string
}

function resolvePatchState(
  isPending: boolean,
  isError: boolean,
  data: NodePatchStatusResource | undefined
): PatchState {
  if (isPending) return 'loading'
  if (isError || !data?.checked) return 'unknown'
  if (data.total === 0) return 'up-to-date'
  if (data.security > 0) return 'security'
  return 'updates'
}

function updatesLabel(data: NodePatchStatusResource | undefined): string {
  if (!data?.checked) return ''
  return `${data.total} update${data.total === 1 ? '' : 's'} available`
}

function securityLabel(data: NodePatchStatusResource | undefined): string {
  if (!data?.checked) return ''
  return `${data.total} update${data.total === 1 ? '' : 's'} available, ${data.security} security`
}

// GET /api/v1/nodes/{id}/patch-status (internal/api/node_patch_status.go):
// this node's latest available-OS-updates reading
// (internal/telemetry/hostpatch.go's HostPatchCollector). A dedicated
// small status card, not a chart: this is one current fact ("3 updates
// available, 1 security"), the same reasoning that handler's own doc
// comment gives for reading the sample directly instead of through the
// generic /metrics time-series query NodeMetricsDashboard uses.
function patchStatusConfig(
  state: PatchState,
  data: NodePatchStatusResource | undefined
): PatchStatusConfig {
  switch (state) {
    case 'loading':
      return { icon: QuestionIcon, variant: 'muted', label: 'Checking...' }
    case 'unknown':
      return { icon: QuestionIcon, variant: 'muted', label: 'Unknown, not checked' }
    case 'up-to-date':
      return { icon: CheckCircleIcon, variant: 'success', label: 'Up to date' }
    case 'security':
      return { icon: WarningCircleIcon, variant: 'destructive', label: securityLabel(data) }
    default:
      return { icon: WarningIcon, variant: 'warning', label: updatesLabel(data) }
  }
}

export function NodePatchStatusCard({ nodeId }: Readonly<{ nodeId: string }>) {
  const { data, isPending, isError } = useNodePatchStatus(nodeId)
  const state = resolvePatchState(isPending, isError, data)
  const { icon: StatusIcon, variant, label } = patchStatusConfig(state, data)

  return (
    <Card>
      <CardHeader>
        <CardTitle>OS patches</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="flex items-center gap-2">
          <Badge variant={variant}>
            <StatusIcon className="size-3" />
            {label}
          </Badge>
          {data?.checked_at ? (
            <span className="text-xs text-muted-foreground">
              Checked {new Date(data.checked_at).toLocaleString()}
            </span>
          ) : null}
        </div>
      </CardContent>
    </Card>
  )
}
