import { createFileRoute } from '@tanstack/react-router'
import {
  CheckCircleIcon,
  CubeIcon,
  MinusCircleIcon,
  StackIcon,
  WarningCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { EmptyState } from '@/components/ui/empty-state'
import { ListSkeleton } from '@/components/ui/list-skeleton'
import { ApiError } from '../../lib/apiError'
import { useContainers } from '../../queries/containers'
import type { ContainerPort, ContainerResource } from '../../queries/containers'

// Web equivalent of "levelrail-cli containers": GET
// /api/v1/system/containers, every container Docker knows about on this
// node whether or not it's managed by this platform. Read-only by
// design (see internal/api/containers.go's own doc comment): a
// Levelrail-managed app is stopped/started/restarted from its own app
// page, which updates desired state correctly, not from a raw
// docker-level action here that would fight the reconciler on the next
// reconcile pass.
export const Route = createFileRoute('/settings/containers')({
  component: ContainersPage,
})

function formatPorts(ports: ContainerPort[]): string {
  if (ports.length === 0) return '-'
  return ports
    .map((p) => `${p.host_port}->${p.container_port}/${p.protocol}`)
    .join(', ')
}

function ContainerRow({ container }: { container: ContainerResource }) {
  return (
    <TableRow>
      <TableCell>
        <Badge variant={container.running ? 'success' : 'muted'}>
          {container.running ? <CheckCircleIcon /> : <MinusCircleIcon />}
          {container.running ? 'Running' : 'Stopped'}
        </Badge>
      </TableCell>
      <TableCell className="font-medium text-foreground">
        {container.name}
      </TableCell>
      <TableCell className="font-mono text-muted-foreground">
        {container.image}
      </TableCell>
      <TableCell className="font-mono text-muted-foreground">
        {formatPorts(container.ports)}
      </TableCell>
    </TableRow>
  )
}

function ContainersPage() {
  const { data: containers, isLoading, isError, error } = useContainers()

  return (
    <div className="space-y-6">
      <div className="flex items-start gap-3">
        <div className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
          <StackIcon className="size-4" />
        </div>
        <div>
          <h1 className="text-lg font-semibold text-foreground">
            Containers
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Every container on this node, whether or not it&apos;s managed by
            this platform.
          </p>
        </div>
      </div>

      {isLoading ? <ListSkeleton rows={5} /> : null}

      {isError ? (
        error instanceof ApiError && error.status === 501 ? (
          <Alert>
            <WarningCircleIcon />
            <AlertTitle>Not available</AlertTitle>
            <AlertDescription>{error.message}</AlertDescription>
          </Alert>
        ) : (
          <Alert variant="destructive">
            <WarningCircleIcon />
            <AlertTitle>Couldn&apos;t list containers</AlertTitle>
            <AlertDescription>{error.message}</AlertDescription>
          </Alert>
        )
      ) : null}

      {!isLoading && !isError && containers ? (
        containers.length === 0 ? (
          <EmptyState
            className="py-12"
            icon={<CubeIcon className="size-5" />}
            title="No containers"
            description="Nothing is running on this node's Docker daemon right now."
          />
        ) : (
          <div className="rounded-lg border border-border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Status</TableHead>
                  <TableHead>Name</TableHead>
                  <TableHead>Image</TableHead>
                  <TableHead>Ports</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {containers.map((container) => (
                  <ContainerRow key={container.name} container={container} />
                ))}
              </TableBody>
            </Table>
          </div>
        )
      ) : null}
    </div>
  )
}
