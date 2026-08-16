import { createFileRoute, Link } from '@tanstack/react-router'
import { z } from 'zod'
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
import { ResetPasswordForm } from '../components/ResetPasswordForm'

const searchSchema = z.object({
  token: z.string().optional().default(''),
})

// Public, unauthenticated: reached from the link in a password-reset
// email (internal/api/password_reset.go's passwordResetURL).
export const Route = createFileRoute('/reset-password')({
  validateSearch: searchSchema,
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(brandQueryOptions()),
  component: ResetPasswordPage,
})

function ResetPasswordPage() {
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
          <CardTitle>Set a new password</CardTitle>
          <CardDescription>
            Choose a new password for the admin account.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {token ? (
            <ResetPasswordForm token={token} />
          ) : (
            <Alert variant="destructive">
              <WarningIcon />
              <AlertDescription>
                This link is missing its reset token. Request a new one from the
                sign-in page.
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
