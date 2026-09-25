import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { useAppLoadBalancerStatus } from '../queries/appLoadBalancer'
import { STATE_VARIANT, summarizePool } from '../lib/loadBalancer'

function formatCheck(iso: string | undefined): string {
  if (!iso) return '-'
  const date = new Date(iso)
  return Number.isNaN(date.getTime()) ? '-' : date.toLocaleTimeString()
}

export function LoadBalancerUpstreamTable({ appName }: { appName: string }) {
  const status = useAppLoadBalancerStatus(appName, true)

  if (status.isLoading) return <Skeleton className="h-24 w-full" />
  if (status.isError) {
    return (
      <p role="alert" className="text-sm text-destructive">
        Could not load upstream status: {status.error.message}
      </p>
    )
  }
  const data = status.data
  if (!data) return null

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-2 text-sm">
        <Badge variant={data.ready ? 'success' : 'warning'}>
          {data.reason}
        </Badge>
        <span className="text-muted-foreground">
          {data.upstreams.length > 0
            ? summarizePool(data.upstreams)
            : data.message}
        </span>
      </div>
      {data.upstreams.length > 0 ? (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Upstream</TableHead>
              <TableHead>State</TableHead>
              <TableHead className="text-right">Weight</TableHead>
              <TableHead className="text-right">Active</TableHead>
              <TableHead className="text-right">Fails</TableHead>
              <TableHead>Last check</TableHead>
              <TableHead>Note</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {data.upstreams.map((u) => (
              <TableRow key={u.id}>
                <TableCell className="font-mono text-xs">
                  {u.dial || `replica ${u.replica}`}
                  {u.node_id ? (
                    <span className="ml-2 text-muted-foreground">
                      node {u.node_id}
                    </span>
                  ) : null}
                </TableCell>
                <TableCell>
                  <Badge variant={STATE_VARIANT[u.state]}>{u.state}</Badge>
                </TableCell>
                <TableCell className="text-right tabular-nums">
                  {u.weight > 0 ? u.weight : '-'}
                </TableCell>
                <TableCell className="text-right tabular-nums">
                  {u.active_connections}
                </TableCell>
                <TableCell className="text-right tabular-nums">
                  {u.fails}
                </TableCell>
                <TableCell className="text-xs text-muted-foreground">
                  {formatCheck(u.last_check)}
                  {u.latency_ms ? ` (${u.latency_ms} ms)` : ''}
                </TableCell>
                <TableCell className="text-xs text-muted-foreground">
                  {u.reason ?? ''}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      ) : null}
    </div>
  )
}
