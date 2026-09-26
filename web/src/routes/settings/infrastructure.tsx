import { useState, type ChangeEvent } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { FileCodeIcon, UploadSimpleIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { IacExportCard } from '../../components/IacExportCard'
import { IacPlanView, IacResultView } from '../../components/IacPlanView'
import { IacIssuesError, useApplyIac, usePlanIac } from '../../queries/iac'
import type { IacFile, IacPlan, IacRequest } from '../../queries/iac'

// Instance level, like the CLI's apply/diff/export: paste or upload resource
// files, review the plan, then apply. Every change inside an apply goes
// through the ordinary API with the signed-in user's own permissions.
export const Route = createFileRoute('/settings/infrastructure')({
  component: InfrastructureSettingsPage,
})

const pastedName = 'pasted.yaml'

function InfrastructureSettingsPage() {
  const planIac = usePlanIac()
  const applyIac = useApplyIac()

  const [pasted, setPasted] = useState('')
  const [uploaded, setUploaded] = useState<IacFile[]>([])
  const [source, setSource] = useState('')
  const [project, setProject] = useState('')
  const [prune, setPrune] = useState(false)
  const [noDeploy, setNoDeploy] = useState(false)
  const [confirming, setConfirming] = useState(false)
  const [reviewed, setReviewed] = useState<IacPlan | null>(null)

  const files: IacFile[] = [
    ...uploaded,
    ...(pasted.trim() ? [{ name: pastedName, content: pasted }] : []),
  ]

  function buildRequest(): IacRequest {
    return {
      files,
      source: source.trim() || undefined,
      project: project.trim() || undefined,
      prune: prune || undefined,
      no_deploy: noDeploy || undefined,
    }
  }

  async function handleUpload(e: ChangeEvent<HTMLInputElement>) {
    const picked = Array.from(e.target.files ?? [])
    const read = await Promise.all(
      picked.map(async (f) => ({ name: f.name, content: await f.text() })),
    )
    setUploaded(read)
    e.target.value = ''
  }

  function handlePlan() {
    applyIac.reset()
    setReviewed(null)
    planIac.mutate(buildRequest(), { onSuccess: (plan) => setReviewed(plan) })
  }

  function handleApply() {
    if (!reviewed) return
    setConfirming(false)
    applyIac.mutate({ ...buildRequest(), expected_plan_hash: reviewed.hash })
  }

  const canPlan = files.length > 0 && !planIac.isPending
  const pruneNeedsSource = prune && !source.trim()
  const pending =
    reviewed !== null &&
    reviewed.summary.create +
      reviewed.summary.update +
      reviewed.summary.delete >
      0
  const canApply =
    pending &&
    reviewed.summary.error === 0 &&
    !applyIac.isPending &&
    !pruneNeedsSource

  return (
    <div className="space-y-6">
      <div className="flex items-start gap-3">
        <div className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
          <FileCodeIcon className="size-4" />
        </div>
        <div>
          <h1 className="text-lg font-semibold text-foreground">
            Infrastructure as code
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Describe projects, environments, apps, domains and databases as
            YAML, review the plan, then apply it.
          </p>
        </div>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Resource files</CardTitle>
          <CardDescription>
            One or more YAML documents separated by ---. Documents never hold
            secret values, reference them with secretRef.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <Field>
            <FieldLabel htmlFor="iac-yaml">YAML</FieldLabel>
            <Textarea
              id="iac-yaml"
              className="min-h-48 font-mono text-xs"
              value={pasted}
              onChange={(e) => setPasted(e.target.value)}
              placeholder={'version: 1\nkind: App\nmetadata:\n  name: web'}
            />
          </Field>
          <div className="flex flex-wrap items-center gap-3">
            <label className="inline-flex cursor-pointer items-center gap-2 text-sm text-foreground">
              <UploadSimpleIcon className="size-4" />
              <span>Upload files</span>
              <input
                type="file"
                multiple
                accept=".yaml,.yml"
                className="sr-only"
                onChange={(e) => void handleUpload(e)}
              />
            </label>
            {uploaded.length > 0 ? (
              <span className="text-xs text-muted-foreground">
                {uploaded.map((f) => f.name).join(', ')}
              </span>
            ) : null}
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <Field>
              <FieldLabel htmlFor="iac-source">Source</FieldLabel>
              <Input
                id="iac-source"
                value={source}
                onChange={(e) => setSource(e.target.value)}
                placeholder="infra-repo"
              />
              <FieldDescription>
                Tags the apps this apply creates so prune can find them later.
              </FieldDescription>
            </Field>
            <Field>
              <FieldLabel htmlFor="iac-project">Only project</FieldLabel>
              <Input
                id="iac-project"
                value={project}
                onChange={(e) => setProject(e.target.value)}
                placeholder="shop"
              />
            </Field>
          </div>
          <div className="space-y-2">
            <label className="flex items-center gap-2 text-sm text-foreground">
              <Checkbox
                checked={prune}
                onCheckedChange={(v) => setPrune(v === true)}
              />
              Prune: delete apps this source created that the files no longer
              declare
            </label>
            {pruneNeedsSource ? (
              <p className="text-xs text-amber-700 dark:text-amber-400">
                Prune needs a source name.
              </p>
            ) : null}
            <label className="flex items-center gap-2 text-sm text-foreground">
              <Checkbox
                checked={noDeploy}
                onCheckedChange={(v) => setNoDeploy(v === true)}
              />
              Do not restart running apps to apply env changes
            </label>
          </div>
          <div className="flex gap-2">
            <Button type="button" disabled={!canPlan} onClick={handlePlan}>
              {planIac.isPending ? 'Planning...' : 'Plan'}
            </Button>
            <Button
              type="button"
              variant="outline"
              disabled={!canApply}
              onClick={() => setConfirming(true)}
            >
              Apply
            </Button>
          </div>
        </CardContent>
      </Card>

      <PlanCard planIac={planIac} reviewed={reviewed} />
      <ResultCard applyIac={applyIac} />
      <IacExportCard />

      <Dialog open={confirming} onOpenChange={setConfirming}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Apply these changes?</DialogTitle>
            <DialogDescription>
              {reviewed
                ? `${reviewed.summary.create} to create, ${reviewed.summary.update} to update, ${reviewed.summary.delete} to delete.`
                : ''}{' '}
              The apply only proceeds if live state still matches the plan you
              reviewed.
            </DialogDescription>
          </DialogHeader>
          {reviewed && reviewed.summary.delete > 0 ? (
            <p className="text-sm text-red-700 dark:text-red-400">
              This deletes {reviewed.summary.delete} resource(s).
            </p>
          ) : null}
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirming(false)}>
              Cancel
            </Button>
            <Button onClick={handleApply}>Apply</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}

function PlanCard({
  planIac,
  reviewed,
}: {
  planIac: ReturnType<typeof usePlanIac>
  reviewed: IacPlan | null
}) {
  const error = planIac.error
  if (!error && !reviewed) return null
  return (
    <Card>
      <CardHeader>
        <CardTitle>Plan</CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        {error instanceof IacIssuesError ? (
          <ul className="space-y-1 font-mono text-sm text-red-700 dark:text-red-400">
            {error.issues.map((i) => (
              <li key={`${i.file}:${i.line}:${i.message}`}>
                {i.file}:{i.line}: {i.message}
                {i.path ? ` (${i.path})` : ''}
              </li>
            ))}
          </ul>
        ) : error ? (
          <p className="text-sm text-red-700 dark:text-red-400">
            {error.message}
          </p>
        ) : reviewed ? (
          <IacPlanView plan={reviewed} />
        ) : null}
      </CardContent>
    </Card>
  )
}

function ResultCard({
  applyIac,
}: {
  applyIac: ReturnType<typeof useApplyIac>
}) {
  const { data, error } = applyIac
  if (!data && !error) return null
  return (
    <Card>
      <CardHeader>
        <CardTitle>Result</CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        {error ? (
          <p className="text-sm text-red-700 dark:text-red-400">
            {error.message}
          </p>
        ) : null}
        {data ? (
          <>
            <IacResultView results={data.results} />
            <p className="text-sm text-muted-foreground">
              {data.applied} applied, {data.failed} failed, {data.skipped}{' '}
              skipped
            </p>
          </>
        ) : null}
      </CardContent>
    </Card>
  )
}
