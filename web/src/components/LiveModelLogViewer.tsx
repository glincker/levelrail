import { useLogStream } from '../hooks/useLogStream'
import { buildLiveModelLogStreamUrl } from '../queries/liveModelLogs'
import { LogConnectionBadge } from './LogConnectionBadge'
import { LogTerminal } from './LogTerminal'

export function LiveModelLogViewer({ modelName }: { modelName: string }) {
  const { lines, connectionState, isPaused, pause, resume } = useLogStream(
    buildLiveModelLogStreamUrl(modelName),
  )
  return (
    <div className="flex h-full flex-col gap-2">
      <div className="flex items-center justify-between gap-3">
        <p className="text-xs text-muted-foreground">
          {lines.length > 0
            ? `${lines.length.toLocaleString()} lines (recent context, then live)`
            : 'Waiting for engine output...'}
        </p>
        <LogConnectionBadge state={connectionState} />
      </div>
      <LogTerminal
        lines={lines}
        isPaused={isPaused}
        pause={pause}
        resume={resume}
        heightClassName="h-[60vh]"
      />
    </div>
  )
}
