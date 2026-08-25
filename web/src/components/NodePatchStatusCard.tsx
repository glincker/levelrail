import {
  CheckCircleIcon,
  QuestionIcon,
  WarningCircleIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import type { Icon } from '@phosphor-icons/react'
import { useNodePatchStatus } from '../queries/nodes'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge, type badgeVariants } from '@/components/ui/badge'
import type { VariantProps } from 'class-variance-authority'

// GET /api/v1/nodes/{id}/patch-status (internal/api/node_patch_status.go):
// this node's latest available-OS-updates reading
// (internal/telemetry/hostpatch.go's HostPatchCollector). A dedicated
// small status card, not a chart: this is one current fact ("3 updates
// available, 1 security"), the same reasoning that handler's own doc
// comment gives for reading the sample directly instead of through the
// generic /metrics time-series query NodeMetricsDashboard uses.
export function NodePatchStatusCard({ nodeId }: { nodeId: string }) {
  const { data, isPending, isError } = useNodePatchStatus(nodeId)

  const state = isPending
    ? 'loading'
    : isError || !data.checked
      ? 'unknown'
      : data.total === 0
        ? 'up-to-date'
        : data.security > 0
          ? 'security'
          : 'updates'

  const config: Record<
    typeof state,
    {
      icon: Icon
      variant: VariantProps<typeof badgeVariants>['variant']
      label: string
    }
  > = {
    loading: { icon: QuestionIcon, variant: 'muted', label: 'Checking...' },
    unknown: {
      icon: QuestionIcon,
      variant: 'muted',
      label: 'Unknown, not checked',
    },
    'up-to-date': {
      icon: CheckCircleIcon,
      variant: 'success',
      label: 'Up to date',
    },
    updates: {
      icon: WarningIcon,
      variant: 'warning',
      label:
        data && data.checked
          ? `${data.total} update${data.total === 1 ? '' : 's'} available`
          : '',
    },
    security: {
      icon: WarningCircleIcon,
      variant: 'destructive',
      label:
        data && data.checked
          ? `${data.total} update${data.total === 1 ? '' : 's'} available, ${data.security} security`
          : '',
    },
  }
  const { icon: StatusIcon, variant, label } = config[state]

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
