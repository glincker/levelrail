import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { FlaskIcon } from '@phosphor-icons/react/dist/ssr'
import { devModeQueryOptions } from '../queries/devMode'
import { useBrand } from '../hooks/useBrand'
import { setupStatusQueryOptions, useLogin } from '../queries/auth'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from './ui/card'
import { Tabs, TabsContent, TabsList, TabsTrigger } from './ui/tabs'
import { Button } from './ui/button'
import { LoginForm } from './LoginForm'
import { RegisterForm } from './RegisterForm'
import { OAuthButtons } from './OAuthButtons'
import { OAuthErrorBanner } from './OAuthErrorBanner'

// internal/api/devmode.go's fixed pair; release builds ignore dev mode entirely (ADR 013).
const DEV_MODE_USERNAME = 'dev'
const DEV_MODE_PASSWORD = 'dev'

type LoginTab = 'sign-in' | 'register'

// The login screen for returning operators and first run alike. The setup
// tab is picked automatically while the instance has no admin, or when the
// installer's ?setup=<token> link was opened.
export function LoginScreen({ setup }: { setup?: string }) {
  const brand = useBrand()
  const setupStatus = useQuery(setupStatusQueryOptions())
  const [chosenTab, setTab] = useState<LoginTab | null>(
    setup ? 'register' : null,
  )
  const tab: LoginTab =
    chosenTab ?? (setupStatus.data?.needs_setup ? 'register' : 'sign-in')
  const brandLabel = brand.ShortName || brand.Name
  const isRegister = tab === 'register'
  // Optional convenience: never blocks or fails the form.
  const devMode = useQuery(devModeQueryOptions())
  const quickLogin = useLogin()

  function handleTabChange(value: unknown): void {
    if (value === 'sign-in' || value === 'register') {
      setTab(value)
    }
  }

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
      <OAuthErrorBanner />
      <Card className="w-full max-w-sm shadow-sm">
        <CardHeader>
          <CardTitle>
            {isRegister ? 'Set up the admin account' : 'Sign in'}
          </CardTitle>
          <CardDescription>
            {isRegister
              ? 'Create the first account for this instance. Registration is for the first account only.'
              : 'Enter your credentials to continue.'}
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <Tabs value={tab} onValueChange={handleTabChange}>
            <TabsList className="grid w-full grid-cols-2">
              <TabsTrigger value="sign-in">Sign in</TabsTrigger>
              <TabsTrigger value="register">Set up admin account</TabsTrigger>
            </TabsList>
            <TabsContent value="sign-in">
              <LoginForm />
            </TabsContent>
            <TabsContent value="register">
              <RegisterForm
                onSwitchToSignIn={() => setTab('sign-in')}
                initialSetupToken={setup ?? ''}
              />
            </TabsContent>
          </Tabs>
          <OAuthButtons />
        </CardContent>
      </Card>
      {devMode.data?.enabled ? (
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="w-full max-w-sm"
          disabled={quickLogin.isPending}
          onClick={() => {
            quickLogin.mutate({
              username: DEV_MODE_USERNAME,
              password: DEV_MODE_PASSWORD,
            })
          }}
        >
          <FlaskIcon />
          {quickLogin.isPending
            ? 'Signing in...'
            : `Quick dev login (${DEV_MODE_USERNAME}/${DEV_MODE_PASSWORD})`}
        </Button>
      ) : null}
    </div>
  )
}
