import { Link } from '@tanstack/react-router'
import {
  PencilSimpleIcon,
  PlusIcon,
  TreeStructureIcon,
  TrashIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { Button, buttonVariants } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { Switch } from '@/components/ui/switch'
import { TableSkeleton } from '@/components/ui/table-skeleton'
import { toast } from '@/components/ui/toast'
import {
  useDeletePipeline,
  usePipeline,
  usePipelines,
  useSavePipeline,
} from '../queries/pipelines'
import type { Pipeline } from '../types/pipelines'
import { PipelineRunsTable } from './PipelineRunsTable'
import { PipelineStatusBadge } from './PipelineStatusBadge'
import { RunPipelineDialog } from './RunPipelineDialog'

function EnabledSwitch({
  appName,
  pipeline,
}: {
  appName: string
  pipeline: Pipeline
}) {
  const save = useSavePipeline(appName)
  const { data: full } = usePipeline(appName, pipeline.name)
  return (
    <Switch
      checked={pipeline.enabled}
      disabled={save.isPending || !full?.yaml}
      aria-label={`Enable ${pipeline.name}`}
      onCheckedChange={(enabled) =>
        save.mutate(
          {
            existing: pipeline.name,
            req: { yaml: full?.yaml ?? '', enabled },
          },
          { onError: (e) => toast.add({ title: e.message, type: 'error' }) },
        )
      }
    />
  )
}

function PipelineRow({
  appName,
  pipeline,
}: {
  appName: string
  pipeline: Pipeline
}) {
  const del = useDeletePipeline(appName)
  return (
    <li className="flex flex-wrap items-center gap-3 rounded-lg border border-border p-3">
      <div className="min-w-48 flex-1">
        <p className="font-medium text-foreground">{pipeline.name}</p>
        <div className="mt-1 flex flex-wrap items-center gap-1.5 text-xs text-muted-foreground">
          {(pipeline.triggers ?? []).map((t) => (
            <Badge key={t} variant="outline">
              {t}
            </Badge>
          ))}
          <span>{pipeline.jobs} jobs</span>
          {pipeline.last_run ? (
            <Link
              to="/apps/$name/pipelines/runs/$runId"
              params={{ name: appName, runId: pipeline.last_run.id }}
              className="inline-flex items-center gap-1"
            >
              #{pipeline.last_run.number}
              <PipelineStatusBadge status={pipeline.last_run.status} />
            </Link>
          ) : (
            <span>never run</span>
          )}
        </div>
      </div>
      <EnabledSwitch appName={appName} pipeline={pipeline} />
      <RunPipelineDialog appName={appName} pipeline={pipeline} />
      <Link
        to="/apps/$name/pipelines/$pipeline"
        params={{ name: appName, pipeline: pipeline.name }}
        className={buttonVariants({ variant: 'outline', size: 'sm' })}
      >
        <PencilSimpleIcon aria-hidden="true" />
        Edit
      </Link>
      <Button
        variant="destructive"
        size="sm"
        disabled={del.isPending}
        onClick={() => {
          if (
            window.confirm(
              `Delete pipeline "${pipeline.name}" and its run history?`,
            )
          ) {
            del.mutate(pipeline.name, {
              onError: (e) => toast.add({ title: e.message, type: 'error' }),
            })
          }
        }}
      >
        <TrashIcon aria-hidden="true" />
        Delete
      </Button>
    </li>
  )
}

export function PipelinesPanel({ appName }: { appName: string }) {
  const { data, isLoading, error } = usePipelines(appName)
  const pipelines = data ?? []
  const newLink = (
    <Link
      to="/apps/$name/pipelines/new"
      params={{ name: appName }}
      className={buttonVariants({ size: 'sm' })}
    >
      <PlusIcon aria-hidden="true" />
      New pipeline
    </Link>
  )
  return (
    <div className="space-y-4">
      <section className="rounded-lg border border-border p-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h2 className="flex items-center gap-1.5 text-sm font-semibold text-foreground">
              <TreeStructureIcon
                className="size-4 text-muted-foreground"
                aria-hidden="true"
              />
              Pipelines
            </h2>
            <p className="mt-1 text-xs text-muted-foreground">
              Test, build, and deploy with approvals, matrix jobs, and
              schedules, defined as YAML.
            </p>
          </div>
          {newLink}
        </div>
        <div className="mt-3">
          {isLoading ? (
            <TableSkeleton columnCount={4} rowCount={2} />
          ) : error ? (
            <p className="text-sm text-destructive">{error.message}</p>
          ) : pipelines.length === 0 ? (
            <EmptyState
              icon={<TreeStructureIcon className="size-5" />}
              title="No pipelines yet"
              description="Define triggers, jobs, and steps in YAML to run tests, build images, and deploy this app."
              action={newLink}
            />
          ) : (
            <ul className="space-y-2">
              {pipelines.map((p) => (
                <PipelineRow key={p.id} appName={appName} pipeline={p} />
              ))}
            </ul>
          )}
        </div>
      </section>
      <section className="rounded-lg border border-border p-4">
        <h2 className="mb-3 text-sm font-semibold text-foreground">
          Run history
        </h2>
        <PipelineRunsTable appName={appName} />
      </section>
    </div>
  )
}
