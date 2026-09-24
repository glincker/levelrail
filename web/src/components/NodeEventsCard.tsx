import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { formatAge } from '../lib/format'
import { useNodeEvents } from '../queries/nodes'
import type { NodeStatus } from '../types/nodeDetail'

const STATUS_VARIANT: Record<NodeStatus, 'success' | 'destructive' | 'muted'> =
  {
    online: 'success',
    offline: 'destructive',
    pending: 'muted',
    cordoned: 'muted',
  }

export function NodeEventsCard({ nodeId }: { nodeId: string }) {
  const { data, isPending, isError } = useNodeEvents(nodeId)

  return (
    <Card>
      <CardHeader>
        <CardTitle>Connection history</CardTitle>
      </CardHeader>
      <CardContent>
        {isPending ? (
          <div
            role="status"
            aria-label="Loading connection history"
            className="h-16 animate-pulse rounded bg-muted motion-reduce:animate-none"
          />
        ) : isError ? (
          <p role="alert" className="text-sm text-muted-foreground">
            History is unavailable right now.
          </p>
        ) : data.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            No status changes recorded yet.
          </p>
        ) : (
          <ul className="space-y-2">
            {data.map((event, i) => (
              <li
                key={`${event.created_at}-${event.to_status}-${i}`}
                className="flex flex-wrap items-center gap-2 text-sm"
              >
                <Badge variant={STATUS_VARIANT[event.from_status]}>
                  {event.from_status}
                </Badge>
                <span>to</span>
                <Badge variant={STATUS_VARIANT[event.to_status]}>
                  {event.to_status}
                </Badge>
                <span
                  className="text-xs text-muted-foreground"
                  title={new Date(event.created_at).toLocaleString()}
                >
                  {formatAge(event.created_at)}
                </span>
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  )
}
