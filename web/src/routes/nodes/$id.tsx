import { createFileRoute, Link, useRouter } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { formatAge } from '../../lib/format'
import {
  nodeDetailQueryOptions,
  nodeHealthQueryOptions,
  nodeListQueryOptions,
  useNode,
  useNodeHealth,
  useSetNodeWorkloads,
} from '../../queries/nodes'
import { ConditionsPanel } from '../../components/ConditionsPanel'
import { NodeDetailHeader } from '../../components/nodes/NodeDetailHeader'
import { NodeStatusCard } from '../../components/nodes/NodeStatusCard'
import { NodeAgentCard } from '../../components/NodeAgentCard'
import { NodeNeverConnectedAlert } from '../../components/NodeNeverConnected'
import { NodeAlertStatusCard } from '../../components/NodeAlertStatusCard'
import { NodeCapacityCard } from '../../components/nodes/NodeCapacityCard'
import { NodeCapacityForecastCard } from '../../components/NodeCapacityForecastCard'
import { NodeGpuCard } from '../../components/NodeGpuCard'
import { NodeDrainsCard } from '../../components/NodeDrainsCard'
import { NodeFailureCard } from '../../components/NodeFailureCard'
import { NodeLabelsCard } from '../../components/NodeLabelsCard'
import { NodeRegionCard } from '../../components/nodes/NodeRegionCard'
import { NodeAllocationCard } from '../../components/nodes/NodeAllocationCard'
import { NodeMeshCard } from '../../components/NodeMeshCard'
import { NodeMetricsDashboard } from '../../components/NodeMetricsDashboard'
import { NodePatchStatusCard } from '../../components/NodePatchStatusCard'
import { NodeEventsCard } from '../../components/NodeEventsCard'
import { routeErrorMessage } from '../../lib/apiError'
import { Button } from '@/components/ui/button'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { InfoTip } from '../../components/kit/InfoTip'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import { Skeleton } from '@/components/ui/skeleton'

// Loader primes the node, its health and the node list (the drain dialog's target picker needs it).
export const Route = createFileRoute('/nodes/$id')({
  loader: ({ context: { queryClient }, params: { id } }) =>
    Promise.all([
      queryClient.ensureQueryData(nodeDetailQueryOptions(id)),
      queryClient.ensureQueryData(nodeHealthQueryOptions(id)),
      queryClient.ensureQueryData(nodeListQueryOptions()),
    ]),
  validateSearch: (search: Record<string, unknown>): { reenroll?: boolean } =>
    search.reenroll === true ||
    search.reenroll === 'true' ||
    search.reenroll === 1 ||
    search.reenroll === '1'
      ? { reenroll: true }
      : {},
  component: NodeDetailPage,
  pendingComponent: NodeDetailSkeleton,
  errorComponent: NodeDetailError,
})

function formatNodeDate(iso: string | undefined, never: string): string {
  return iso ? new Date(iso).toLocaleString() : never
}

function NodeDetailPage() {
  const { t } = useTranslation('nodes')
  const { id } = Route.useParams()
  const { reenroll } = Route.useSearch()
  const { data: node } = useNode(id)
  const { data: conditions } = useNodeHealth(id)
  const setWorkloads = useSetNodeWorkloads()

  return (
    <NodeDetailHeader node={node}>
      <NodeStatusCard node={node} />

      <Card>
        <CardHeader>
          <CardTitle>{t('detail.overview.title')}</CardTitle>
        </CardHeader>
        <CardContent>
          <dl className="grid grid-cols-2 gap-4 sm:grid-cols-3">
            <div>
              <dt className="text-xs text-muted-foreground uppercase">
                {t('detail.overview.address')}
              </dt>
              <dd className="mt-1 break-all font-mono text-sm text-foreground">
                {node.address ?? t('detail.overview.notSet')}
              </dd>
            </div>
            <div>
              <dt className="text-xs text-muted-foreground uppercase">
                {t('detail.overview.joined')}
              </dt>
              <dd className="mt-1 text-sm text-foreground">
                {formatNodeDate(node.joined_at, t('detail.overview.never'))}
              </dd>
            </div>
            <div>
              <dt className="text-xs text-muted-foreground uppercase">
                {t('detail.overview.lastSeen')}
              </dt>
              <dd
                className="mt-1 text-sm text-foreground"
                title={formatNodeDate(
                  node.last_seen_at,
                  t('detail.overview.never'),
                )}
              >
                {formatAge(node.last_seen_at, t('detail.overview.never'))}
              </dd>
            </div>
          </dl>
        </CardContent>
      </Card>

      <NodeRegionCard key={node.id} node={node} />

      <NodeAgentCard node={node} openReenroll={reenroll === true} />

      <Card>
        <CardHeader>
          <CardTitle>{t('detail.workloads.title')}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <Field orientation="horizontal">
            <Switch
              id="node-accepts-app-workloads"
              checked={node.accepts_app_workloads}
              disabled={setWorkloads.isPending}
              onCheckedChange={(checked) => {
                setWorkloads.mutate(
                  {
                    id: node.id,
                    acceptsAppWorkloads: checked,
                    acceptsBuildWorkloads: node.accepts_build_workloads,
                  },
                  {
                    onSuccess: () => {
                      toast.add({
                        title: checked
                          ? t('detail.workloads.appsOn')
                          : t('detail.workloads.appsOff'),
                        type: 'success',
                      })
                    },
                  },
                )
              }}
            />
            <FieldLabel htmlFor="node-accepts-app-workloads">
              {t('detail.workloads.apps')}
            </FieldLabel>
          </Field>
          <FieldDescription>{t('detail.workloads.appsHint')}</FieldDescription>

          <Field orientation="horizontal">
            <Switch
              id="node-accepts-build-workloads"
              checked={node.accepts_build_workloads}
              disabled={setWorkloads.isPending}
              onCheckedChange={(checked) => {
                setWorkloads.mutate(
                  {
                    id: node.id,
                    acceptsAppWorkloads: node.accepts_app_workloads,
                    acceptsBuildWorkloads: checked,
                  },
                  {
                    onSuccess: () => {
                      toast.add({
                        title: checked
                          ? t('detail.workloads.buildsOn')
                          : t('detail.workloads.buildsOff'),
                        type: 'success',
                      })
                    },
                  },
                )
              }}
            />
            <FieldLabel
              htmlFor="node-accepts-build-workloads"
              className="flex items-center gap-1.5"
            >
              {t('detail.workloads.builds')}
              <InfoTip label={t('detail.workloads.buildsTipLabel')}>
                {t('detail.workloads.buildsTip')}
              </InfoTip>
            </FieldLabel>
          </Field>
          <FieldDescription>
            {t('detail.workloads.buildsHint')}
          </FieldDescription>

          {setWorkloads.isError ? (
            <Alert variant="destructive">
              <AlertDescription>{setWorkloads.error.message}</AlertDescription>
            </Alert>
          ) : null}
        </CardContent>
      </Card>

      <NodeNeverConnectedAlert node={node} />
      <NodeAlertStatusCard nodeId={id} />

      <NodeGpuCard gpu={node.gpu} />

      <NodeMeshCard nodeId={id} nodeName={node.name} />

      <NodeAllocationCard nodeId={id} />
      <NodeLabelsCard nodeId={id} />
      <NodeDrainsCard nodeId={id} />
      <NodeFailureCard nodeId={id} />

      <NodePatchStatusCard nodeId={id} />
      <NodeEventsCard nodeId={id} />
      <NodeCapacityCard nodeId={id} />
      <NodeCapacityForecastCard nodeId={id} />

      <NodeMetricsDashboard nodeId={id} />

      <ConditionsPanel conditions={conditions} />
    </NodeDetailHeader>
  )
}

function NodeDetailSkeleton() {
  return (
    <div className="space-y-6" aria-hidden="true">
      <div>
        <Skeleton className="h-3 w-14" />
        <div className="mt-1 flex flex-wrap items-center justify-between gap-3">
          <div className="flex flex-wrap items-center gap-2">
            <Skeleton className="h-6 w-36" />
            <Skeleton className="h-5 w-16 rounded-full" />
          </div>
          <div className="flex items-center gap-2">
            <Skeleton className="h-9 w-20 rounded-md" />
            <Skeleton className="h-9 w-20 rounded-md" />
          </div>
        </div>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>
            <Skeleton className="h-4 w-20" />
          </CardTitle>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-2 gap-4 sm:grid-cols-3">
            {Array.from({ length: 3 }, (_, i) => (
              <div key={i} className="space-y-1">
                <Skeleton className="h-3 w-16" />
                <Skeleton className="h-4 w-24" />
              </div>
            ))}
          </div>
        </CardContent>
      </Card>

      {Array.from({ length: 8 }, (_, i) => (
        <Card key={i}>
          <CardHeader>
            <CardTitle>
              <Skeleton className="h-4 w-28" />
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-2">
            <Skeleton className="h-4 w-full" />
            <Skeleton className="h-4 w-2/3" />
          </CardContent>
        </Card>
      ))}
    </div>
  )
}

function NodeDetailError({ error }: { error: unknown }) {
  const { t } = useTranslation('nodes')
  const router = useRouter()
  return (
    <Alert variant="destructive">
      <AlertDescription className="space-y-3">
        <p>{routeErrorMessage(error)}</p>
        <div className="flex flex-wrap gap-2">
          <Button
            type="button"
            size="sm"
            onClick={() => {
              void router.invalidate()
            }}
          >
            {t('detail.retry')}
          </Button>
          <Button
            size="sm"
            variant="outline"
            render={<Link to="/nodes" />}
            nativeButton={false}
          >
            {t('detail.back')}
          </Button>
        </div>
      </AlertDescription>
    </Alert>
  )
}
