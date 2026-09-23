import { Link } from '@tanstack/react-router'
import { LockOpenIcon } from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { isInsecureRemoteConnection } from '../lib/connection'
import { DASHBOARD_URL_ANCHOR } from './DashboardUrlCard'

// Shown on every page while the dashboard is served over plain HTTP.
// Deliberately not dismissible: the risk persists until HTTPS is set up.
export function InsecureConnectionBanner() {
  if (!isInsecureRemoteConnection(window.location)) {
    return null
  }
  return (
    <Alert className="rounded-none border-x-0 border-t-0 border-amber-200 bg-amber-50 text-amber-900 dark:border-amber-900/50 dark:bg-amber-900/20 dark:text-amber-200">
      <LockOpenIcon />
      <AlertTitle>This connection is not encrypted</AlertTitle>
      <AlertDescription>
        Your password and session travel in plain text. Point a domain at this
        server and{' '}
        <Link to="/domains" hash={DASHBOARD_URL_ANCHOR}>
          set up an https dashboard URL
        </Link>
        .
      </AlertDescription>
    </Alert>
  )
}
