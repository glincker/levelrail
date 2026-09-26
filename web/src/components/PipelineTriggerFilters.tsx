import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { toast } from '@/components/ui/toast'
import { useApplyPipelineFilters } from '../queries/pipelines'
import type { PipelineFilters } from '../types/pipelines'
import { InfoTip } from './kit'

function toLines(list: string[]): string {
  return list.join('\n')
}

function fromLines(text: string): string[] {
  return text
    .split('\n')
    .map((l) => l.trim())
    .filter((l) => l !== '')
}

// PipelineTriggerFilters edits the path filters on a pipeline's push and
// pull_request triggers, and its report_status flag, by rewriting the YAML
// through the server so the file stays the single source of truth.
export function PipelineTriggerFilters({
  yaml,
  filters,
  onApply,
}: {
  yaml: string
  filters: PipelineFilters
  onApply: (next: string) => void
}) {
  const [paths, setPaths] = useState(toLines(filters.paths))
  const [ignore, setIgnore] = useState(toLines(filters.paths_ignore))
  const [report, setReport] = useState(filters.report_status)
  const apply = useApplyPipelineFilters()

  const dirty =
    paths !== toLines(filters.paths) ||
    ignore !== toLines(filters.paths_ignore) ||
    report !== filters.report_status

  const submit = () =>
    apply.mutate(
      {
        yaml,
        paths: fromLines(paths),
        paths_ignore: fromLines(ignore),
        report_status: report,
      },
      {
        onSuccess: onApply,
        onError: (e) =>
          toast.add({
            title: 'Could not apply the filters.',
            description: e.message,
            type: 'error',
          }),
      },
    )

  return (
    <fieldset className="space-y-3 rounded-lg border border-border p-3">
      <legend className="px-1 text-sm font-medium text-foreground">
        Trigger filters
      </legend>
      <div className="grid gap-4 sm:grid-cols-2">
        <div className="space-y-1.5">
          <Label htmlFor="pipeline-paths" className="flex items-center gap-1.5">
            Only run for changes to
            <InfoTip label="About paths">
              One glob per line, such as src/** or **/*.go. The run starts only
              when a changed file matches at least one. Applies to push and pull
              request triggers. Leave empty to run for every change.
            </InfoTip>
          </Label>
          <Textarea
            id="pipeline-paths"
            value={paths}
            onChange={(e) => setPaths(e.target.value)}
            rows={3}
            spellCheck={false}
            className="font-mono text-xs"
            placeholder="src/**"
          />
        </div>
        <div className="space-y-1.5">
          <Label
            htmlFor="pipeline-paths-ignore"
            className="flex items-center gap-1.5"
          >
            Skip when only these change
            <InfoTip label="About paths_ignore">
              One glob per line, such as docs/** or **/*.md. A change is skipped
              when every changed file matches. Use it to exclude files an
              included glob would otherwise pick up.
            </InfoTip>
          </Label>
          <Textarea
            id="pipeline-paths-ignore"
            value={ignore}
            onChange={(e) => setIgnore(e.target.value)}
            rows={3}
            spellCheck={false}
            className="font-mono text-xs"
            placeholder="docs/**"
          />
        </div>
      </div>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <Switch
            id="pipeline-report-status"
            checked={report}
            onCheckedChange={setReport}
            aria-label="Report status to the git provider"
          />
          <Label
            htmlFor="pipeline-report-status"
            className="flex items-center gap-1.5"
          >
            Report status to the git provider
            <InfoTip label="About status reporting">
              Posts the run state (pending, success, failure) as a commit status
              linking back to the run. A failed post never fails the run. The
              server can switch this off for every app with
              APP_GIT_STATUS_ENABLED=false.
            </InfoTip>
          </Label>
        </div>
        <Button
          size="sm"
          variant="outline"
          onClick={submit}
          disabled={!dirty || apply.isPending}
        >
          {apply.isPending ? 'Applying...' : 'Apply to YAML'}
        </Button>
      </div>
    </fieldset>
  )
}
