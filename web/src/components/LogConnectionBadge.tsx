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

// "open" and "error" source their color from the shared success/warning
// badge variants (components/ui/badge.tsx) instead of duplicating
// bg-green-100/bg-amber-100 locally. "connecting" has no green/red/amber
// equivalent in badgeVariants, it is a neutral state, so it keeps its
// literal Tailwind classes.
const CONNECTING_BADGE_CLASS =
  'bg-neutral-100 text-neutral-700 dark:bg-neutral-800 dark:text-neutral-300'

const CONNECTION_VARIANT: Record<
  Exclude<LogStreamConnectionState, 'connecting'>,
  'success' | 'warning'
> = {
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
    <Badge
      variant={state === 'connecting' ? undefined : CONNECTION_VARIANT[state]}
      className={`gap-1.5 rounded-full ${
        state === 'connecting' ? CONNECTING_BADGE_CLASS : ''
      }`}
    >
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
