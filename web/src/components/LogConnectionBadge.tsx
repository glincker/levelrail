import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import type { LogStreamConnectionState } from '../hooks/useLogStream'

// Shared connection-state indicator for both live SSE log viewers (deploy
// logs and running-app container logs). notRunning overrides the
// connection state: an open stream with no running container is not "Live".

type BadgeState = LogStreamConnectionState | 'notRunning'

const CONNECTION_LABEL_KEY: Record<
  BadgeState,
  | 'connection.connecting'
  | 'connection.open'
  | 'connection.error'
  | 'connection.notRunning'
> = {
  connecting: 'connection.connecting',
  open: 'connection.open',
  error: 'connection.error',
  notRunning: 'connection.notRunning',
}

const CONNECTION_VARIANT: Record<
  BadgeState,
  'success' | 'warning' | 'muted' | 'destructive'
> = {
  connecting: 'muted',
  open: 'success',
  error: 'warning',
  notRunning: 'destructive',
}

// The dot pulses only while genuinely live; a still dot reads as "not live".
const CONNECTION_DOT_CLASS: Record<BadgeState, string> = {
  connecting: 'bg-neutral-500 dark:bg-neutral-400',
  open: 'bg-green-600 dark:bg-green-400',
  error: 'bg-amber-600 dark:bg-amber-400',
  notRunning: 'bg-tone-danger-solid',
}

export function LogConnectionBadge({
  state,
  notRunning = false,
}: {
  state: LogStreamConnectionState
  notRunning?: boolean
}) {
  const { t } = useTranslation('observability')
  const shown: BadgeState = notRunning ? 'notRunning' : state
  return (
    <Badge variant={CONNECTION_VARIANT[shown]} className="gap-1.5 rounded-full">
      <span className="relative inline-flex size-2" aria-hidden="true">
        {shown === 'open' ? (
          <span
            className={`absolute inline-flex size-full animate-ping rounded-full opacity-75 ${CONNECTION_DOT_CLASS[shown]}`}
          />
        ) : null}
        <span
          className={`relative inline-flex size-2 rounded-full ${CONNECTION_DOT_CLASS[shown]}`}
        />
      </span>
      {t(CONNECTION_LABEL_KEY[shown])}
    </Badge>
  )
}
