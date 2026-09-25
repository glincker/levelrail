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
}: {
  app: string
  runId: string
  job: string
}) {
  const { data, isLoading, error } = usePipelineRunLogs(app, runId, job)
  const [paused, setPaused] = useState(false)
  const lines = useMemo<LogLine[]>(
    () =>
      (data ?? []).map((l) => ({ id: l.id, line: l.line, stream: l.stream })),
    [data],
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
      isFinished
    />
  )
}

// Follows a running job's output over SSE; a finished run reads the stored
// lines once, so a completed page never holds an event stream open.
export function PipelineRunLogs({
  app,
  runId,
  job,
  live,
}: {
  app: string
  runId: string
  job: string
  live: boolean
}) {
  return live ? (
    <LiveLogs key={`${runId}/${job}`} app={app} runId={runId} job={job} />
  ) : (
    <StoredLogs key={`${runId}/${job}`} app={app} runId={runId} job={job} />
  )
}
