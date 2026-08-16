import { createFileRoute, Link } from '@tanstack/react-router'
import { ArrowLeftIcon } from '@phosphor-icons/react/dist/ssr'
import { brandQueryOptions } from '../queries/brand'
import { useBrand } from '../hooks/useBrand'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '../components/ui/card'
import { ForgotPasswordForm } from '../components/ForgotPasswordForm'

// Public, unauthenticated, reached from LoginForm.tsx's "Forgot
// password?" link.
export const Route = createFileRoute('/forgot-password')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(brandQueryOptions()),
  component: ForgotPasswordPage,
})

function ForgotPasswordPage() {
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
          <CardTitle>Reset your password</CardTitle>
          <CardDescription>
            We'll email a reset link if the address matches an account.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <ForgotPasswordForm />
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
