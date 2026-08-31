import {
  CheckCircleIcon,
  QuestionIcon,
  WarningCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import type { Icon } from '@phosphor-icons/react'
import { useNode } from '../queries/nodes'
import type { NodeAlertState } from '../types/nodeDetail'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge, type badgeVariants } from '@/components/ui/badge'
import type { VariantProps } from 'class-variance-authority'

type AlertStatusRow = {
  label: string
  state: NodeAlertState
}

const stateConfig: Record<
  NodeAlertState,
  { icon: Icon; variant: VariantProps<typeof badgeVariants>['variant']; label: string }
> = {
  ok: { icon: CheckCircleIcon, variant: 'success', label: 'OK' },
  firing: { icon: WarningCircleIcon, variant: 'destructive', label: 'Firing' },
  unknown: { icon: QuestionIcon, variant: 'muted', label: 'Unknown' },
}

// GET /api/v1/nodes/{id}'s alert_status (internal/api/nodes.go's
// handleGetNode): a live re-evaluation of whether patch_status,
// node_disk_space, or node_resource_usage is currently firing because of
// this specific node, not a rule's stored aggregate LastValue (which only
// ever names the worst node across the whole fleet, never which one).
// Absent entirely when telemetry isn't configured on this control plane.
export function NodeAlertStatusCard({ nodeId }: Readonly<{ nodeId: string }>) {
  const { data } = useNode(nodeId)
  const status = data.alert_status
  if (!status) return null

  const rows: AlertStatusRow[] = [
    { label: 'Patch status', state: status.patch_status },
    { label: 'Disk space', state: status.node_disk_space },
    { label: 'Resource usage', state: status.node_resource_usage },
  ]

  return (
    <Card>
      <CardHeader>
        <CardTitle>Alert status</CardTitle>
      </CardHeader>
      <CardContent>
        <dl className="grid grid-cols-1 gap-2 sm:grid-cols-3">
          {rows.map((row) => {
            const { icon: StatusIcon, variant, label } = stateConfig[row.state]
            return (
              <div key={row.label} className="flex items-center justify-between gap-2">
                <dt className="text-sm text-muted-foreground">{row.label}</dt>
                <dd>
                  <Badge variant={variant}>
                    <StatusIcon className="size-3" />
                    {label}
                  </Badge>
                </dd>
              </div>
            )
          })}
        </dl>
      </CardContent>
    </Card>
  )
}
