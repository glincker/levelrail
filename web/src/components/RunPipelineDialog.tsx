import { useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { PlayIcon, WarningIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { useStartPipelineRun } from '../queries/pipelines'
import type { Pipeline } from '../types/pipelines'
import { parseInputLines } from '../lib/pipelineTemplate'

export function RunPipelineDialog({
  appName,
  pipeline,
}: {
  appName: string
  pipeline: Pipeline
}) {
  const [open, setOpen] = useState(false)
  const [ref, setRef] = useState('')
  const [sha, setSha] = useState('')
  const [inputs, setInputs] = useState('')
  const start = useStartPipelineRun(appName)
  const navigate = useNavigate()
  const canRun =
    pipeline.enabled &&
    (pipeline.triggers ?? []).some((t) => t === 'manual' || t === 'api')

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        setOpen(next)
        if (!next) {
          start.reset()
        }
      }}
    >
      <DialogTrigger
        render={<Button size="sm" disabled={!canRun} />}
        title={
          canRun
            ? undefined
            : 'Enable the pipeline and a manual trigger to run it'
        }
      >
        <PlayIcon aria-hidden="true" />
        Run
      </DialogTrigger>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Run {pipeline.name}</DialogTitle>
          <DialogDescription>
            Starts a run now. Leave ref and commit empty to use the
            repository&apos;s default branch head.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <div className="space-y-1.5">
            <Label htmlFor="run-ref">Ref</Label>
            <Input
              id="run-ref"
              value={ref}
              onChange={(e) => setRef(e.target.value)}
              placeholder="refs/heads/main"
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="run-sha">Commit SHA</Label>
            <Input
              id="run-sha"
              value={sha}
              onChange={(e) => setSha(e.target.value)}
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="run-inputs">Inputs (one key=value per line)</Label>
            <Textarea
              id="run-inputs"
              value={inputs}
              onChange={(e) => setInputs(e.target.value)}
              className="font-mono"
              placeholder="env=staging"
            />
          </div>
        </div>
        {start.isError ? (
          <Alert variant="destructive">
            <WarningIcon />
            <AlertDescription>{start.error.message}</AlertDescription>
          </Alert>
        ) : null}
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => setOpen(false)}
          >
            Cancel
          </Button>
          <Button
            type="button"
            disabled={start.isPending}
            onClick={() =>
              start.mutate(
                {
                  name: pipeline.name,
                  req: { ref, sha, inputs: parseInputLines(inputs) },
                },
                {
                  onSuccess: (run) => {
                    setOpen(false)
                    void navigate({
                      to: '/apps/$name/pipelines/runs/$runId',
                      params: { name: appName, runId: run.id },
                    })
                  },
                },
              )
            }
          >
            {start.isPending ? 'Starting...' : 'Start run'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
