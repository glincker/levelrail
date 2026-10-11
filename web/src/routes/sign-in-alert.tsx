import { createFileRoute, Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  CheckCircleIcon,
  ShieldWarningIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import { brandQueryOptions } from '../queries/brand'
import { useDisownSignIn } from '../queries/securityCenter'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '../components/ui/card'
import { Alert, AlertDescription } from '../components/ui/alert'
import { Button, buttonVariants } from '../components/ui/button'

interface SignInAlertSearch {
  token: string
}

// Plain function, not zod, so validateSearch stays out of the eager bundle.
function validateSignInAlertSearch(
  search: Record<string, unknown>,
): SignInAlertSearch {
  const { token } = search
  return { token: typeof token === 'string' ? token : '' }
}

// Public: reached from the new sign-in email. Opening the link changes
// nothing; only the button does, so mail scanners cannot trigger it.
export const Route = createFileRoute('/sign-in-alert')({
  validateSearch: validateSignInAlertSearch,
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(brandQueryOptions()),
  component: SignInAlertPage,
})

function SignInAlertPage() {
  const { t } = useTranslation('security')
  const { token } = Route.useSearch()
  const disown = useDisownSignIn()
  return (
    <div className="flex min-h-[70vh] items-center justify-center px-4">
      <Card className="w-full max-w-md shadow-sm">
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <ShieldWarningIcon className="size-5" aria-hidden="true" />
            {t('alertPage.title')}
          </CardTitle>
          <CardDescription>{t('alertPage.description')}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {!token ? (
            <Alert variant="destructive">
              <WarningIcon />
              <AlertDescription>{t('alertPage.missing')}</AlertDescription>
            </Alert>
          ) : null}
          {disown.isError ? (
            <Alert variant="destructive">
              <WarningIcon />
              <AlertDescription>{t('alertPage.error')}</AlertDescription>
            </Alert>
          ) : null}
          {disown.isSuccess ? (
            <Alert>
              <CheckCircleIcon />
              <AlertDescription>
                {disown.data.session_revoked
                  ? t('alertPage.done')
                  : t('alertPage.doneNoSession')}
              </AlertDescription>
            </Alert>
          ) : (
            <Button
              type="button"
              variant="destructive"
              className="w-full"
              disabled={!token || disown.isPending}
              onClick={() => {
                disown.mutate(token)
              }}
            >
              {disown.isPending
                ? t('alertPage.working')
                : t('alertPage.confirm')}
            </Button>
          )}
          <Link
            to="/login"
            className={buttonVariants({ variant: 'link', size: 'sm' })}
          >
            {t('alertPage.signIn')}
          </Link>
        </CardContent>
      </Card>
    </div>
  )
}
