import { Link } from '@tanstack/react-router'
import { LockOpenGlassIcon } from '@thesvg/ui-react'
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
    <Alert className="rounded-none border-x-0 border-t-0 border-tone-warning-border bg-tone-warning-soft text-tone-warning">
      <LockOpenGlassIcon size="1.2em" />
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
