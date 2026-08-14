import { createFileRoute, redirect } from '@tanstack/react-router'
import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { FlaskIcon } from '@phosphor-icons/react/dist/ssr'
import { getStoredUsername } from '../lib/authStore'
import { brandQueryOptions } from '../queries/brand'
import { devModeQueryOptions } from '../queries/devMode'
import { useBrand } from '../hooks/useBrand'
import { useLogin } from '../queries/auth'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '../components/ui/card'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '../components/ui/tabs'
import { Button } from '../components/ui/button'
import { LoginForm } from '../components/LoginForm'
import { RegisterForm } from '../components/RegisterForm'

// Matches internal/api/devmode.go's devModeUsername/devModePassword
// exactly (also the same pair dev-fixtures.yml documents at the repo
// root). Fixed and deliberately not secret, same reasoning ADR 013
// gives for not randomizing them: the backend's build-tag gate
// (devmode_debug.go vs devmode_release.go) is what makes this safe, not
// credential secrecy, so hardcoding the known pair here to save a click
// is not a new risk on top of what already exists server-side.
const DEV_MODE_USERNAME = 'dev'
const DEV_MODE_PASSWORD = 'dev'

type LoginTab = 'sign-in' | 'register'

// The one login route, handling both the returning-operator and the
// first-run cases (docs-local/research/dashboard-gap-audit-and-devmode.md
// gap #1 and #3). There is deliberately no "does an admin already exist"
// signal from the backend, so this does not auto-detect first-run and
// render a different screen for it; it's one screen with a tab toggle the
// operator picks themselves, a real, intentional deviation from
// auto-detection, not an oversight (see TASKS.md's note on this task for
// the full reasoning).
//
// Already-authenticated visitors get bounced to /apps: this route's own
// job is only ever reached when there's no session to speak of.
export const Route = createFileRoute('/login')({
  beforeLoad: () => {
    if (getStoredUsername() !== null) {
      redirect({ to: '/apps', throw: true })
    }
  },
  // The login screen needs branding before a session exists, per the
  // rebrandability rule that no product name is hardcoded in components,
  // same as the root route's own loader; primed again here so
  // this route works even if it's the very first thing to load (root's
  // beforeLoad redirect can land here before root's own loader runs on a
  // fresh navigation, so this route can't assume the cache is warm yet).
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(brandQueryOptions()),
  component: LoginPage,
})

function LoginPage() {
  const brand = useBrand()
  const [tab, setTab] = useState<LoginTab>('sign-in')
  const brandLabel = brand.ShortName || brand.Name
  const isRegister = tab === 'register'
  // Not suspense: this is a purely optional convenience, the real login
  // form above must always render immediately regardless of whether this
  // query has resolved yet, is slow, or fails outright (isError is
  // deliberately not checked below, a failed check just means the button
  // stays hidden, exactly like enabled being false).
  const devMode = useQuery(devModeQueryOptions())
  const quickLogin = useLogin()

  function handleTabChange(value: unknown): void {
    if (value === 'sign-in' || value === 'register') {
      setTab(value)
    }
  }

  return (
    <div className="flex min-h-[70vh] flex-col items-center justify-center gap-6 px-4">
      {/* Brand mark above the card: this is the first screen anyone sees,
          so it gets a small identity anchor rather than just dropping
          straight into a bare form (see the visual-pass task's framing
          of the login screen as a trust signal). Renders the brand's
          initial rather than brand.LogoSVG: nothing else in this repo
          renders that field yet (grep confirms), so there is no
          established sanitize-and-inject pattern to reuse here, and a
          visual pass is not the place to invent one. */}
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
          <CardTitle>
            {isRegister ? 'Set up the admin account' : 'Sign in'}
          </CardTitle>
          <CardDescription>
            {isRegister
              ? 'Create the one admin account for this instance. Registration is for the first admin only.'
              : 'Enter your admin credentials to continue.'}
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Tabs value={tab} onValueChange={handleTabChange}>
            <TabsList className="grid w-full grid-cols-2">
              <TabsTrigger value="sign-in">Sign in</TabsTrigger>
              <TabsTrigger value="register">Set up admin account</TabsTrigger>
            </TabsList>
            <TabsContent value="sign-in">
              <LoginForm />
            </TabsContent>
            <TabsContent value="register">
              <RegisterForm onSwitchToSignIn={() => setTab('sign-in')} />
            </TabsContent>
          </Tabs>
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
