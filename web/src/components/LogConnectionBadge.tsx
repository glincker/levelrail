import { Badge } from '@/components/ui/badge'
import type { LogStreamConnectionState } from '../hooks/useLogStream'

// Shared connection-state indicator for both live SSE log viewers
// (deploy/build logs and running-app container logs, see
// hooks/useLogStream.ts's own doc comment on why one hook now backs
// both). Extracted out of the deploy logs route, where this originated,
// rather than left duplicated once a second live viewer needed the
// identical badge: same three states, same "never hide a connection
// problem from the user" requirement either way.

const CONNECTION_LABEL: Record<LogStreamConnectionState, string> = {
  connecting: 'Connecting...',
  open: 'Live',
  error: 'Reconnecting...',
}

// All three states source their color from badgeVariants
// (components/ui/badge.tsx) instead of duplicating its classes locally.
const CONNECTION_VARIANT: Record<
  LogStreamConnectionState,
  'success' | 'warning' | 'muted'
> = {
  connecting: 'muted',
  open: 'success',
  error: 'warning',
}

// The connection dot pulses only while genuinely live (SSE stream open):
// a still dot for "Connecting..."/"Reconnecting..." would read as "also
// live", the opposite of what those two states mean.
const CONNECTION_DOT_CLASS: Record<LogStreamConnectionState, string> = {
  connecting: 'bg-neutral-500 dark:bg-neutral-400',
  open: 'bg-green-600 dark:bg-green-400',
  error: 'bg-amber-600 dark:bg-amber-400',
}

export function LogConnectionBadge({
  state,
}: {
  state: LogStreamConnectionState
}) {
  return (
    <Badge variant={CONNECTION_VARIANT[state]} className="gap-1.5 rounded-full">
      <span className="relative inline-flex size-2" aria-hidden="true">
        {state === 'open' ? (
          <span
            className={`absolute inline-flex size-full animate-ping rounded-full opacity-75 ${CONNECTION_DOT_CLASS[state]}`}
          />
        ) : null}
        <span
          className={`relative inline-flex size-2 rounded-full ${CONNECTION_DOT_CLASS[state]}`}
        />
      </span>
      {CONNECTION_LABEL[state]}
    </Badge>
  )
}
