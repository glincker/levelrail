import * as React from 'react'
import { Link } from '@tanstack/react-router'
import { WarningCircleIcon, XIcon } from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertAction, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { useDockerHealthPoll } from '../queries/systemStatus'

// Dashboard-wide, impossible-to-miss surface for the one outage that
// makes every other page's own error text misleading: a per-app "cannot
// connect to the Docker daemon" message reads as that app's problem,
// when it is actually the platform's. Lives in routes/__root.tsx's
// AppShell, above every route's own content, so it shows regardless of
// which page happens to be open when Docker goes down.
//
// Dismissing only clears the current outage, not the underlying signal:
// dismissed resets to false the moment docker_connected flips back to
// true, so a later, separate outage shows its own fresh banner rather
// than staying silenced by a dismissal from hours or days earlier.
export function DockerHealthBanner() {
  const { data } = useDockerHealthPoll()
  const [dismissed, setDismissed] = React.useState(false)
  // "No data yet" reads as connected, the same default the component's
  // early return below already treats as nothing-to-show, so the very
  // first render before the initial fetch resolves never flashes a banner.
  const connected = data?.docker_connected ?? true

  // Reset the dismissal the moment Docker recovers, adjusted directly
  // during render (React's documented alternative to an effect for
  // "reset state when a prop changes") rather than in a useEffect: a
  // later, separate outage then shows its own fresh banner instead of
  // staying silenced by a dismissal from hours or days earlier.
  const [prevConnected, setPrevConnected] = React.useState(connected)
  if (connected !== prevConnected) {
    setPrevConnected(connected)
    if (connected) {
      setDismissed(false)
    }
  }

  if (!data || connected || dismissed) {
    return null
  }

  return (
    <Alert variant="destructive" className="rounded-none border-x-0 border-t-0">
      <WarningCircleIcon />
      <AlertTitle>Docker Engine is unreachable</AlertTitle>
      <AlertDescription>
        This node cannot reach its Docker daemon. Deploys, restarts,
        rollbacks, exec, and log streaming will fail until it is back.
        {data.docker_error ? ` (${data.docker_error})` : null}{' '}
        <Link to="/nodes">Check node status</Link>.
      </AlertDescription>
      <AlertAction>
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          aria-label="Dismiss for now"
          title="Dismiss for now"
          onClick={() => setDismissed(true)}
        >
          <XIcon />
        </Button>
      </AlertAction>
    </Alert>
  )
}
