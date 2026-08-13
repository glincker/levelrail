import { createFileRoute } from '@tanstack/react-router'
import { useState } from 'react'
import {
  CheckCircle2Icon,
  LogOutIcon,
  ShieldCheckIcon,
  TriangleAlertIcon,
} from 'lucide-react'
import {
  sessionQueryOptions,
  useRevokeOtherSessions,
  useSession,
} from '../../queries/security'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'

// Loader-primed the same way routes/settings/tokens.tsx primes
// tokenListQueryOptions: the component below only ever reads that warm
// cache via useSuspenseQuery, never fetches in its own body (CLAUDE.md 7).
export const Route = createFileRoute('/settings/security')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(sessionQueryOptions()),
  component: SecuritySettingsPage,
})

// Matches TokenTable.tsx's own formatDate convention: toLocaleString(),
// no separate date-formatting library, same as every other date already
// rendered in this app.
function formatDate(iso: string): string {
  return new Date(iso).toLocaleString()
}

function SecuritySettingsPage() {
  const { data: session } = useSession()

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-lg font-semibold text-foreground">Security</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Sessions and login protection.
        </p>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Current session</CardTitle>
          <CardDescription>
            The account and session this browser is signed in with.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-1.5 text-sm">
          <p>
            <span className="text-muted-foreground">Signed in as </span>
            <span className="font-medium text-foreground">
              {session.username}
            </span>
          </p>
          <p>
            <span className="text-muted-foreground">Signed in until </span>
            <span className="font-medium text-foreground">
              {formatDate(session.expires_at)}
            </span>
          </p>
        </CardContent>
      </Card>

      <OtherSessionsCard />

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <ShieldCheckIcon className="size-4 text-muted-foreground" />
            Login protection
          </CardTitle>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-muted-foreground">
            Failed sign-in attempts are rate limited per IP address and
            username, with each additional failure making the next allowed
            attempt wait longer than the last. A handful of mistyped passwords
            will never lock you out, but repeated automated guessing gets slower
            with every try.
          </p>
        </CardContent>
      </Card>
    </div>
  )
}

// Own card, own dialog: sits between the read-only "current session"
// card and the static login-protection panel, matching
// RevokeTokenDialog.tsx's confirm-then-mutate shape for the same
// reason, this is a real, meaningful, if not strictly irreversible,
// security action (every other live session for this admin account is
// ended), not a one-click toggle.
function OtherSessionsCard() {
  const [open, setOpen] = useState(false)
  const [justRevoked, setJustRevoked] = useState(false)
  const revokeOtherSessions = useRevokeOtherSessions()

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      revokeOtherSessions.reset()
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Other sessions</CardTitle>
        <CardDescription>
          Sign out every other browser or device currently signed in to this
          account, without changing your password. This browser stays signed in.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        {justRevoked ? (
          <Alert>
            <CheckCircle2Icon className="text-green-700 dark:text-green-400" />
            <AlertTitle>Other sessions signed out</AlertTitle>
            <AlertDescription>
              Every other session for this account has been ended. This browser
              was not affected.
            </AlertDescription>
          </Alert>
        ) : null}
        <Dialog open={open} onOpenChange={handleOpenChange}>
          <DialogTrigger render={<Button variant="outline" />}>
            <LogOutIcon />
            Sign out other sessions
          </DialogTrigger>
          <DialogContent className="sm:max-w-sm">
            <DialogHeader>
              <DialogTitle className="flex items-center gap-2">
                <TriangleAlertIcon className="size-4 text-destructive" />
                Sign out other sessions?
              </DialogTitle>
              <DialogDescription>
                Any other browser or device currently signed in to this account
                will be signed out immediately. This browser is not affected.
              </DialogDescription>
            </DialogHeader>
            {revokeOtherSessions.isError ? (
              <Alert variant="destructive">
                <TriangleAlertIcon />
                <AlertDescription>
                  {revokeOtherSessions.error.message}
                </AlertDescription>
              </Alert>
            ) : null}
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => {
                  handleOpenChange(false)
                }}
              >
                Cancel
              </Button>
              <Button
                type="button"
                disabled={revokeOtherSessions.isPending}
                onClick={() => {
                  revokeOtherSessions.mutate(undefined, {
                    onSuccess: () => {
                      setOpen(false)
                      setJustRevoked(true)
                    },
                  })
                }}
              >
                {revokeOtherSessions.isPending
                  ? 'Signing out...'
                  : 'Sign out other sessions'}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </CardContent>
    </Card>
  )
}
