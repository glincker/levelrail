import { Link } from '@tanstack/react-router'
import { ClockCounterClockwiseIcon } from '@phosphor-icons/react/dist/ssr'
import { useAppConfigActivity } from '../queries/auditLog'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'

function formatDate(iso: string): string {
  return new Date(iso).toLocaleString()
}

// Deliberately generic: the audit log records that a PUT reached this
// app's config endpoint, its actor, and its timestamp, never a
// field-level diff (internal/store/audit.go never stores request
// bodies, so it has nothing to diff and can never leak an env or secret
// value). "Configuration updated" is all this panel can honestly say.
export function EnvActivityPanel({ appName }: { appName: string }) {
  const { data, isLoading, isError } = useAppConfigActivity(appName, 5)

  // A non-root actor (or a transient failure) fails this secondary panel
  // quietly rather than surfacing an error state on top of the editor
  // itself, which the operator can still use either way.
  if (isError) {
    return null
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <ClockCounterClockwiseIcon className="size-4" />
          Recent activity
        </CardTitle>
        <CardDescription>
          Configuration changes to this app, newest first. See{' '}
          <Link
            to="/settings/audit-log"
            className="underline underline-offset-2"
          >
            Settings &gt; Audit log
          </Link>{' '}
          for the full trail across every resource.
        </CardDescription>
      </CardHeader>
      <CardContent>
        {isLoading ? (
          <p className="text-sm text-muted-foreground">Loading...</p>
        ) : data && data.length > 0 ? (
          <ul className="space-y-2">
            {data.map((entry) => (
              <li
                key={entry.id}
                className="flex flex-wrap items-center gap-2 text-sm"
              >
                <span className="text-muted-foreground">
                  {formatDate(entry.created_at)}
                </span>
                <span className="font-medium text-foreground">
                  {entry.actor_name}
                </span>
                <Badge variant="outline">Configuration updated</Badge>
              </li>
            ))}
          </ul>
        ) : (
          <p className="text-sm text-muted-foreground">
            No recorded configuration changes yet.
          </p>
        )}
      </CardContent>
    </Card>
  )
}
