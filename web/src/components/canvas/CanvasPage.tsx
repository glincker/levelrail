import { useMemo, useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { LayoutGridGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { buttonVariants } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { toast } from '@/components/ui/toast'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { cn } from '@/lib/utils'
import { Breadcrumbs } from '../Breadcrumbs'
import { InfraCanvas } from '../topology/InfraCanvas'
import { useCanvasReview } from '../../hooks/useCanvasReview'
import { useExperimentalFeatures } from '../../hooks/useExperimental'
import { BETA_FEATURES_PATH, lockedSearch } from '../../lib/experimental'
import {
  addConnection,
  effectiveGraph,
  hasPlanOps,
  removeConnection,
  removeOp,
  type DraftOp,
} from '../../lib/canvasDraft'
import { useLiveProjectTopology } from '../../queries/projectTopology'
import { useProject } from '../../queries/projects'
import { CanvasDraftPanel } from './CanvasDraftPanel'
import { CanvasReviewDialog } from './CanvasReviewDialog'
import { AddAppDialog, AddDatabaseDialog } from './CanvasForms'

let opCounter = 0
const nextId = () => `op-${++opCounter}`

export function CanvasPage({ projectId }: { projectId: string }) {
  const { t } = useTranslation('canvas')
  const { data: project } = useProject(projectId)
  const { data: live } = useLiveProjectTopology(projectId)
  const iacOn = useExperimentalFeatures().includes('iac')
  const [ops, setOps] = useState<DraftOp[]>([])
  const [addApp, setAddApp] = useState(false)
  const [addDb, setAddDb] = useState(false)

  const graph = useMemo(() => effectiveGraph(live, ops), [live, ops])
  const review = useCanvasReview(projectId, project.name, ops)

  async function onApply() {
    const result = await review.apply().catch(() => null)
    if (!result) return
    toast.add(
      result.ok
        ? { title: t('toast.applied'), type: 'success' }
        : { title: t('toast.failed'), type: 'error' },
    )
    if (result.ok) setOps([])
  }

  return (
    <PageTemplate
      breadcrumb={<Breadcrumbs projectId={projectId} />}
      title={t('title', { name: project.name })}
      description={t('description')}
      icon={<LayoutGridGlassIcon size="1.2em" />}
      status={<Badge variant="info">{t('beta')}</Badge>}
      width="full"
      aside={
        <CanvasDraftPanel
          graph={graph}
          ops={ops}
          canReview={ops.length > 0 && iacOn}
          onAddApp={() => setAddApp(true)}
          onAddDatabase={() => setAddDb(true)}
          onConnect={(app, database, envVar) =>
            setOps((cur) =>
              addConnection(cur, live, app, database, envVar, nextId()),
            )
          }
          onDisconnect={(app, database) =>
            setOps((cur) => removeConnection(cur, app, database, nextId()))
          }
          onRemoveOp={(id) => setOps((cur) => removeOp(cur, id))}
          onDiscard={() => setOps([])}
          onReview={review.review}
        />
      }
      asideLabel={t('asideLabel')}
    >
      {iacOn ? null : (
        <Alert className="mb-4">
          <AlertDescription className="space-y-2">
            <p className="font-medium">{t('iacOff.title')}</p>
            <p>{t('iacOff.description')}</p>
            <Link
              to={BETA_FEATURES_PATH}
              search={lockedSearch('iac', `/projects/${projectId}/canvas`)}
              className={cn(buttonVariants({ variant: 'outline', size: 'sm' }))}
            >
              {t('iacOff.action')}
            </Link>
          </AlertDescription>
        </Alert>
      )}

      {graph.nodes.length === 0 ? (
        <EmptyState
          icon={<LayoutGridGlassIcon className="size-5.5" />}
          title={t('empty.title')}
          description={t('empty.description')}
        />
      ) : (
        <InfraCanvas projectId={projectId} graph={graph} />
      )}

      <AddAppDialog
        open={addApp}
        graph={graph}
        onOpenChange={setAddApp}
        onAdd={(name, image, port) => {
          setOps((cur) => [
            ...cur,
            { id: nextId(), type: 'add-app', name, image, port },
          ])
          setAddApp(false)
        }}
      />
      <AddDatabaseDialog
        open={addDb}
        graph={graph}
        onOpenChange={setAddDb}
        onAdd={(name, engine, version) => {
          setOps((cur) => [
            ...cur,
            { id: nextId(), type: 'add-database', name, engine, version },
          ])
          setAddDb(false)
        }}
      />
      <CanvasReviewDialog
        open={review.open}
        hasServerPlan={hasPlanOps(ops)}
        plan={review.plan}
        planning={review.planning}
        planError={review.planError}
        applyError={review.applyError}
        applying={review.applying}
        outcome={review.outcome}
        resolved={review.resolved}
        lookupsLoading={review.lookupsLoading}
        onClose={review.close}
        onRetry={review.review}
        onApply={() => void onApply()}
      />
    </PageTemplate>
  )
}
