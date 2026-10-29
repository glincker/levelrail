import { useSyncExternalStore } from 'react'
import {
  LoaderCircleGlassIcon,
  WifiOffGlassIcon,
  XGlassIcon,
} from '@thesvg/ui-react'
import {
  Alert,
  AlertAction,
  AlertDescription,
  AlertTitle,
} from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  dismissConnectionBanner,
  getConnectionSnapshot,
  retryNow,
  subscribeConnection,
} from '../lib/connectionStore'

export function ConnectionBanner() {
  const state = useSyncExternalStore(
    subscribeConnection,
    getConnectionSnapshot,
    getConnectionSnapshot,
  )

  if (!state.offline || state.dismissed) {
    return null
  }

  return (
    <Alert variant="destructive" className="rounded-none border-x-0 border-t-0">
      <WifiOffGlassIcon size="1.2em" />
      <AlertTitle>Can&apos;t reach the control plane. Retrying...</AlertTitle>
      <AlertDescription>
        <span role="status" aria-live="polite">
          {state.probing
            ? 'Checking the connection now.'
            : `Reconnect attempts so far: ${String(state.attempt)}. Live updates are paused until it is back.`}
        </span>
        <span className="mt-2 block">
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={state.probing}
            onClick={() => {
              void retryNow()
            }}
          >
            {state.probing ? (
              <LoaderCircleGlassIcon
                size="1.2em"
                className="motion-safe:animate-spin"
              />
            ) : null}
            Retry now
          </Button>
        </span>
      </AlertDescription>
      <AlertAction>
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          aria-label="Dismiss for now"
          title="Dismiss for now"
          onClick={dismissConnectionBanner}
        >
          <XGlassIcon size="1.2em" />
        </Button>
      </AlertAction>
    </Alert>
  )
}
