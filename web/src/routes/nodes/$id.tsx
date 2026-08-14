import { createFileRoute, Link } from '@tanstack/react-router'
import { ArrowLeftIcon } from '@phosphor-icons/react/dist/ssr'
import {
  nodeDetailQueryOptions,
  nodeHealthQueryOptions,
  nodeListQueryOptions,
  useNode,
  useNodeHealth,
  useSetNodeWorkloads,
} from '../../queries/nodes'
import type { NodeStatus } from '../../types/nodeDetail'
import { ConditionsPanel } from '../../components/ConditionsPanel'
import { CordonNodeDialog } from '../../components/CordonNodeDialog'
import { DrainNodeDialog } from '../../components/DrainNodeDialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge, type badgeVariants } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Switch } from '@/components/ui/switch'
import type { VariantProps } from 'class-variance-authority'

// Node detail route, mirroring routes/databases/$name.tsx's shape: three
// queries primed in the loader in parallel (the node itself, its health
// conditions, and the full node list). The node list is needed here
// too, not just on /nodes, because DrainNodeDialog's target-node picker
// (useNodes() internally) must have real data the moment this route
// renders, not just when an operator has already visited /nodes first
// in the same session.
//
// This route exists because cordon/drain/health all produce genuinely
// multi-line per-node content (a conditions list, a drain result listing
// every moved/failed resource) that does not fit a list row, unlike the
// plain status+address+last-seen fields NodeRow already showed inline.
// Delete stays on the list row (NodeRow.tsx, unchanged): unlike
// cordon/drain, delete needed no new state or multi-line result to show,
// so there was no structural reason to move it, and DeleteNodeDialog.tsx
// itself is left untouched per this task's own file-boundary note.
export const Route = createFileRoute('/nodes/$id')({
  loader: ({ context: { queryClient }, params: { id } }) =>
    Promise.all([
      queryClient.ensureQueryData(nodeDetailQueryOptions(id)),
      queryClient.ensureQueryData(nodeHealthQueryOptions(id)),
      queryClient.ensureQueryData(nodeListQueryOptions()),
    ]),
  component: NodeDetailPage,
  errorComponent: NodeDetailError,
})

// Same two-axis reasoning NodeRow.tsx documents: connectivity (Status)
// and cordon (Schedulable) are independent, and nothing in this codebase
// currently sets status to "cordoned" (internal/store/nodes.go's own
// doc comment on NodeStatusCordoned), so this page renders both badges
// separately rather than special-casing a status value that never
// actually appears on the wire.
const STATUS_BADGE_VARIANT: Record<
  NodeStatus,
  VariantProps<typeof badgeVariants>['variant']
> = {
  pending: 'muted',
  online: 'success',
  offline: 'destructive',
  cordoned: 'warning',
}

const STATUS_LABEL: Record<NodeStatus, string> = {
  pending: 'Pending',
  online: 'Online',
  offline: 'Offline',
  cordoned: 'Cordoned',
}

function formatNodeDate(iso?: string): string {
  if (!iso) {
    return 'Never'
  }
  return new Date(iso).toLocaleString()
}

function NodeDetailPage() {
  const { id } = Route.useParams()
  const { data: node } = useNode(id)
  const { data: conditions } = useNodeHealth(id)
  const setWorkloads = useSetNodeWorkloads()

  return (
    <div className="space-y-6">
      <div>
        <Link
          to="/nodes"
          className="inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground"
        >
          <ArrowLeftIcon className="size-3" />
          Nodes
        </Link>
        <div className="mt-1 flex flex-wrap items-center justify-between gap-3">
          <div className="flex flex-wrap items-center gap-2">
            <h1 className="text-lg font-semibold text-foreground">
              {node.name}
            </h1>
            <Badge variant={STATUS_BADGE_VARIANT[node.status]}>
              {STATUS_LABEL[node.status]}
            </Badge>
            {node.schedulable ? null : (
              <Badge variant="warning">Cordoned</Badge>
            )}
          </div>
          <div className="flex items-center gap-2">
            <CordonNodeDialog node={node} />
            <DrainNodeDialog node={node} />
          </div>
        </div>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Overview</CardTitle>
        </CardHeader>
        <CardContent>
          <dl className="grid grid-cols-2 gap-4 sm:grid-cols-3">
            <div>
              <dt className="text-xs text-muted-foreground uppercase">
                Address
              </dt>
              <dd className="mt-1 font-mono text-sm text-foreground">
                {node.address ?? 'Not set'}
              </dd>
            </div>
            <div>
              <dt className="text-xs text-muted-foreground uppercase">
                Joined
              </dt>
              <dd className="mt-1 text-sm text-foreground">
                {formatNodeDate(node.joined_at)}
              </dd>
            </div>
            <div>
              <dt className="text-xs text-muted-foreground uppercase">
                Last seen
              </dt>
              <dd className="mt-1 text-sm text-foreground">
                {formatNodeDate(node.last_seen_at)}
              </dd>
            </div>
          </dl>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Workload capabilities</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <Field orientation="horizontal">
            <Switch
              id="node-accepts-app-workloads"
              checked={node.accepts_app_workloads}
              disabled={setWorkloads.isPending}
              onCheckedChange={(checked) => {
                setWorkloads.mutate({
                  id: node.id,
                  acceptsAppWorkloads: checked,
                  acceptsBuildWorkloads: node.accepts_build_workloads,
                })
              }}
            />
            <FieldLabel htmlFor="node-accepts-app-workloads">
              Accepts app workloads
            </FieldLabel>
          </Field>
          <FieldDescription>
            Whether this node is eligible for new application placements.
          </FieldDescription>

          <Field orientation="horizontal">
            <Switch
              id="node-accepts-build-workloads"
              checked={node.accepts_build_workloads}
              disabled={setWorkloads.isPending}
              onCheckedChange={(checked) => {
                setWorkloads.mutate({
                  id: node.id,
                  acceptsAppWorkloads: node.accepts_app_workloads,
                  acceptsBuildWorkloads: checked,
                })
              }}
            />
            <FieldLabel htmlFor="node-accepts-build-workloads">
              Accepts build workloads
            </FieldLabel>
          </Field>
          <FieldDescription>
            Whether this node is considered for dedicated build placement
            (internal/build.SelectBuildNode).
          </FieldDescription>

          {setWorkloads.isError ? (
            <p className="text-sm text-destructive">
              {setWorkloads.error.message}
            </p>
          ) : null}
        </CardContent>
      </Card>

      <ConditionsPanel conditions={conditions} />
    </div>
  )
}

// fetchNode (queries/nodes.ts) throws a plain ApiError for a 404, which
// lands here rather than crashing the whole route tree, mirroring
// DatabaseDetailError.
function NodeDetailError({ error }: { error: Error }) {
  return (
    <Alert variant="destructive">
      <AlertDescription>
        <p>{error.message}</p>
        <Link to="/nodes" className="mt-2 inline-block underline">
          Back to nodes
        </Link>
      </AlertDescription>
    </Alert>
  )
}
