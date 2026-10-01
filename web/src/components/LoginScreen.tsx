import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { FlaskIcon } from '@phosphor-icons/react/dist/ssr'
import { devModeQueryOptions } from '../queries/devMode'
import { useBrand } from '../hooks/useBrand'
import { setupStatusQueryOptions, useLogin } from '../queries/auth'
import { getLastUsername } from '../lib/authStore'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from './ui/card'
import { Tabs, TabsContent, TabsList, TabsTrigger } from './ui/tabs'
import { Button } from './ui/button'
import { RegisterForm } from './RegisterForm'
import { OAuthButtons } from './OAuthButtons'
import { OAuthErrorBanner } from './OAuthErrorBanner'
import { SignInFlow } from './SignInFlow'
import { InsecureConnectionBanner } from './InsecureConnectionBanner'
import { ThemeToggle } from './ThemeToggle'
import { BrandMarkGlyph } from './BrandMarkGlyph'

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
  // handleRegister 409s once any account exists, so hide the tab once
  // setup is confirmed done rather than offer a dead end.
  const showRegisterTab =
    Boolean(setup) || setupStatus.data?.needs_setup !== false
  const [username, setUsername] = useState(getLastUsername)
  // Optional convenience: never blocks or fails the form.
  const devMode = useQuery(devModeQueryOptions())
  const quickLogin = useLogin()

  function handleTabChange(value: unknown): void {
    if (value === 'sign-in' || value === 'register') {
      setTab(value)
    }
  }

  const signInContent = (
    <SignInFlow username={username} onUsernameChange={setUsername} />
  )

  return (
    <div className="relative flex min-h-[70vh] flex-col items-center justify-center gap-6 overflow-hidden px-4">
      {/* Decorative only: a soft amber wash reusing the docs site's own
          "rail signal" brand accent, scoped to this page, not the
          app-wide shadcn tokens. */}
      <div
        aria-hidden="true"
        className="pointer-events-none absolute inset-x-0 top-0 -z-10 h-[32rem]"
        style={{
          background:
            'radial-gradient(60% 55% at 50% 0%, rgb(245 158 11 / 0.12), transparent 70%)',
        }}
      />
      <div className="absolute top-4 right-4">
        <ThemeToggle />
      </div>
      <div className="flex flex-col items-center gap-2 text-center">
        <div
          aria-hidden="true"
          className="flex size-10 items-center justify-center rounded-lg bg-foreground text-base font-semibold text-background shadow-[0_0_0_4px_rgb(245_158_11_/_0.12)]"
        >
          <BrandMarkGlyph />
        </div>
        <span className="text-sm font-medium text-foreground">
          {brandLabel}
        </span>
      </div>
      <div className="w-full max-w-sm">
        <InsecureConnectionBanner />
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
          {showRegisterTab ? (
            <Tabs value={tab} onValueChange={handleTabChange}>
              <TabsList className="grid w-full grid-cols-2">
                <TabsTrigger value="sign-in">Sign in</TabsTrigger>
                <TabsTrigger value="register">Set up admin account</TabsTrigger>
              </TabsList>
              <TabsContent value="sign-in" className="space-y-3">
                {signInContent}
              </TabsContent>
              <TabsContent value="register">
                <RegisterForm
                  onSwitchToSignIn={() => setTab('sign-in')}
                  initialSetupToken={setup ?? ''}
                />
              </TabsContent>
            </Tabs>
          ) : (
            <div className="space-y-3">{signInContent}</div>
          )}
          <OAuthButtons />
        </CardContent>
      </Card>
      {devMode.data?.enabled ? (
        <div className="w-full max-w-sm space-y-2">
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="w-full"
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
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="w-full text-muted-foreground"
            onClick={() => {
              setUsername(DEV_MODE_USERNAME)
            }}
          >
            Fill username only (test the sign-in flow above)
          </Button>
          <p className="text-center text-xs text-muted-foreground">
            To test the passkey branch: register one for {DEV_MODE_USERNAME} in
            Settings, then use Chrome DevTools &rsaquo; More tools &rsaquo;
            WebAuthn to add a virtual authenticator.
          </p>
        </div>
      ) : null}
    </div>
  )
}
