import { Link } from '@tanstack/react-router'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { TableSkeleton } from '@/components/ui/table-skeleton'
import { usePipelineRuns } from '../queries/pipelines'
import { formatDuration } from '../lib/pipelineStatus'
import { PipelineStatusBadge } from './PipelineStatusBadge'

export function PipelineRunsTable({
  appName,
  pipeline,
}: {
  appName: string
  pipeline?: string
}) {
  const { data, isLoading, error } = usePipelineRuns(appName, pipeline)
  if (isLoading) {
    return <TableSkeleton columnCount={6} rowCount={4} />
  }
  if (error) {
    return <p className="text-sm text-destructive">{error.message}</p>
  }
  const runs = data ?? []
  if (runs.length === 0) {
    return <p className="text-sm text-muted-foreground">No runs yet.</p>
  }
  return (
    <div className="rounded-lg border border-border">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Run</TableHead>
            <TableHead>Status</TableHead>
            <TableHead>Trigger</TableHead>
            <TableHead>Ref</TableHead>
            <TableHead>Started</TableHead>
            <TableHead>Duration</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {runs.map((r) => (
            <TableRow key={r.id}>
              <TableCell>
                <Link
                  to="/apps/$name/pipelines/runs/$runId"
                  params={{ name: appName, runId: r.id }}
                  className="font-medium text-foreground underline-offset-2 hover:underline"
                >
                  {r.pipeline_name} #{r.number}
                </Link>
              </TableCell>
              <TableCell>
                <PipelineStatusBadge status={r.status} />
              </TableCell>
              <TableCell className="text-xs text-muted-foreground">
                {r.trigger}
                {r.actor ? ` by ${r.actor}` : ''}
              </TableCell>
              <TableCell className="max-w-48 truncate font-mono text-xs text-muted-foreground">
                {r.ref ?? ''}
              </TableCell>
              <TableCell className="text-xs text-muted-foreground">
                {new Date(r.created_at).toLocaleString()}
              </TableCell>
              <TableCell className="text-xs tabular-nums text-muted-foreground">
                {formatDuration(r.started_at, r.finished_at)}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}
