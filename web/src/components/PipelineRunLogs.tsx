import { useMemo, useState } from 'react'
import { useLogStream, type LogLine } from '../hooks/useLogStream'
import { LogTerminal } from './LogTerminal'
import { pipelineLogStreamUrl, usePipelineRunLogs } from '../queries/pipelines'

function LiveLogs({
  app,
  runId,
  job,
}: {
  app: string
  runId: string
  job: string
}) {
  const { lines, isPaused, pause, resume } = useLogStream(
    pipelineLogStreamUrl(app, runId, job),
  )
  return (
    <LogTerminal
      lines={lines}
      isPaused={isPaused}
      pause={pause}
      resume={resume}
      heightClassName="h-96"
      emptyStateMessage="Waiting for output..."
    />
  )
}

function StoredLogs({
  app,
  runId,
  job,
  step,
  live,
}: {
  app: string
  runId: string
  job: string
  step?: number
  live: boolean
}) {
  const { data, isLoading, error } = usePipelineRunLogs(app, runId, job, live)
  const [paused, setPaused] = useState(false)
  const lines = useMemo<LogLine[]>(
    () =>
      (data ?? [])
        .filter((l) => step === undefined || l.step === step)
        .map((l) => ({ id: l.id, line: l.line, stream: l.stream })),
    [data, step],
  )
  if (error) {
    return <p className="text-sm text-destructive">{error.message}</p>
  }
  return (
    <LogTerminal
      lines={lines}
      isPaused={paused}
      pause={() => setPaused(true)}
      resume={() => setPaused(false)}
      heightClassName="h-96"
      emptyStateMessage={isLoading ? 'Loading logs...' : 'No output recorded.'}
      emptyStatePulse={false}
      isFinished={!live}
    />
  )
}

// A whole running job follows its output over SSE. A finished run, or one
// step of any run (the stream carries no step filter), reads the stored
// lines, polling while the run is live, so a completed page never holds an
// event stream open.
export function PipelineRunLogs({
  app,
  runId,
  job,
  step,
  live,
}: {
  app: string
  runId: string
  job: string
  step?: number
  live: boolean
}) {
  if (live && step === undefined) {
    return (
      <LiveLogs key={`${runId}/${job}`} app={app} runId={runId} job={job} />
    )
  }
  return (
    <StoredLogs
      key={`${runId}/${job}/${step ?? 'all'}`}
      app={app}
      runId={runId}
      job={job}
      step={step}
      live={live}
    />
  )
}
