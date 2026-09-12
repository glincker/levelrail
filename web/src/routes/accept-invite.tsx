import { createFileRoute, Link } from '@tanstack/react-router'
import { ArrowLeftIcon, WarningIcon } from '@phosphor-icons/react/dist/ssr'
import { brandQueryOptions } from '../queries/brand'
import { useBrand } from '../hooks/useBrand'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '../components/ui/card'
import { Alert, AlertDescription } from '../components/ui/alert'
import { AcceptInviteForm } from '../components/AcceptInviteForm'

interface AcceptInviteSearch {
  token: string
}

// A plain function, not a zod schema: same reasoning
// routes/reset-password.tsx's own validateResetPasswordSearch documents,
// to keep zod's ~100kB out of the eagerly-loaded route tree.
function validateAcceptInviteSearch(
  search: Record<string, unknown>,
): AcceptInviteSearch {
  const { token } = search
  return { token: typeof token === 'string' ? token : '' }
}

// Public, unauthenticated: reached from the link in a team-invite email
// or the copy/paste link (internal/api/invites.go's inviteURL).
export const Route = createFileRoute('/accept-invite')({
  validateSearch: validateAcceptInviteSearch,
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(brandQueryOptions()),
  component: AcceptInvitePage,
})

function AcceptInvitePage() {
  const { token } = Route.useSearch()
  const brand = useBrand()
  const brandLabel = brand.ShortName || brand.Name

  return (
    <div className="flex min-h-[70vh] flex-col items-center justify-center gap-6 px-4">
      <div className="flex flex-col items-center gap-2 text-center">
        <div
          aria-hidden="true"
          className="flex size-10 items-center justify-center rounded-lg bg-foreground text-base font-semibold text-background"
        >
          {brandLabel.charAt(0).toUpperCase()}
        </div>
        <span className="text-sm font-medium text-foreground">
          {brandLabel}
        </span>
      </div>
      <Card className="w-full max-w-sm shadow-sm">
        <CardHeader>
          <CardTitle>Accept your invite</CardTitle>
          <CardDescription>
            Choose a password to finish creating your account.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {token ? (
            <AcceptInviteForm token={token} />
          ) : (
            <Alert variant="destructive">
              <WarningIcon />
              <AlertDescription>
                This link is missing its invite token. Ask whoever invited you
                to send a new one.
              </AlertDescription>
            </Alert>
          )}
          <Link
            to="/login"
            className="flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
          >
            <ArrowLeftIcon className="size-3.5" />
            Back to sign in
          </Link>
        </CardContent>
      </Card>
    </div>
  )
}
