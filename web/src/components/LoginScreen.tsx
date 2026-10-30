import { useEffect, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  FlaskConicalGlassIcon,
  LogOutGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import { devModeQueryOptions } from '../queries/devMode'
import { useBrand } from '../hooks/useBrand'
import {
  setupStatusQueryOptions,
  useConsumeSessionLink,
  useLogin,
} from '../queries/auth'
import { getLastUsername } from '../lib/authStore'
import { Card, CardContent } from './ui/card'
import { Tabs, TabsContent, TabsList, TabsTrigger } from './ui/tabs'
import { Button } from './ui/button'
import { Alert, AlertDescription } from './ui/alert'
import { DemoLoginEntry } from './demo/DemoLoginEntry'
import { RegisterForm } from './RegisterForm'
import { OAuthButtons } from './OAuthButtons'
import { OAuthErrorBanner } from './OAuthErrorBanner'
import { SignInFlow } from './SignInFlow'
import { InsecureConnectionBanner } from './InsecureConnectionBanner'
import { ThemeToggle } from './ThemeToggle'
import { BrandMarkGlyph } from './BrandMarkGlyph'
import { LoginBrandPanel } from './LoginBrandPanel'
import { LockedOutDialog } from './LockedOutDialog'
import { LoginAttribution } from './LoginAttribution'
import { Display, Eyebrow, Lede } from './ui/typography'

// internal/api/devmode.go's fixed pair; release builds ignore dev mode entirely (ADR 013).
const DEV_MODE_USERNAME = 'dev'
const DEV_MODE_PASSWORD = 'dev'

type LoginTab = 'sign-in' | 'register'

// The login screen for returning operators and first run alike. The setup
// tab is picked automatically while the instance has no admin, or when the
// installer's #setup=<token> link was opened. sessionLink, when present
// (a ?session_link=<token> link from "levelrail-cli auth session-link"
// or POST /api/v1/auth/session-links), is consumed automatically on
// mount instead of showing the normal sign-in form at all.
export function LoginScreen({
  setup,
  sessionLink,
  resumeApprovalId,
  sessionEnded = false,
}: {
  setup?: string
  sessionLink?: string
  resumeApprovalId?: string
  sessionEnded?: boolean
}) {
  const brand = useBrand()
  const { t } = useTranslation('common')
  const setupStatus = useQuery(setupStatusQueryOptions())
  const consumeSessionLink = useConsumeSessionLink()
  const sessionLinkStarted = useRef(false)
  useEffect(() => {
    if (!sessionLink || sessionLinkStarted.current) {
      return
    }
    sessionLinkStarted.current = true
    consumeSessionLink.mutate(sessionLink)
  }, [sessionLink, consumeSessionLink])
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
    <SignInFlow
      username={username}
      onUsernameChange={setUsername}
      resumeApprovalId={resumeApprovalId}
    />
  )

  if (sessionLink) {
    return (
      <div className="flex min-h-[70vh] flex-col items-center justify-center gap-6 px-4">
        <div className="flex flex-col items-center gap-2 text-center">
          <div
            aria-hidden="true"
            className="flex size-10 items-center justify-center rounded-lg text-base font-semibold shadow-[0_0_0_4px_rgb(245_158_11_/_0.12)]"
          >
            <BrandMarkGlyph />
          </div>
          <span className="text-sm font-medium text-foreground">
            {brandLabel}
          </span>
        </div>
        <Card className="w-full max-w-sm shadow-sm">
          <CardContent className="space-y-4 pt-6">
            {consumeSessionLink.isError ? (
              <>
                <Alert variant="destructive">
                  <TriangleAlertGlassIcon
                    className="text-tone-warning"
                    size="1.7em"
                  />
                  <AlertDescription>
                    {t('sessionLink.invalid')}
                  </AlertDescription>
                </Alert>
                <Link
                  to="/login"
                  className="block text-center text-sm text-muted-foreground hover:text-foreground"
                >
                  {t('sessionLink.backToSignIn')}
                </Link>
              </>
            ) : (
              <p className="text-center text-sm text-muted-foreground">
                {t('sessionLink.signingIn')}
              </p>
            )}
          </CardContent>
        </Card>
      </div>
    )
  }

  return (
    <div className="grid min-h-dvh bg-background lg:grid-cols-[minmax(0,1fr)_minmax(0,1.15fr)]">
      <div className="relative flex min-w-0 flex-col px-6 pt-[max(1.5rem,env(safe-area-inset-top))] pb-[max(1.5rem,env(safe-area-inset-bottom))] sm:px-10">
        <header className="relative z-10 flex items-center justify-between">
          <div className="flex items-center gap-2.5">
            <span
              aria-hidden="true"
              className="flex size-8 items-center justify-center rounded-lg"
            >
              <BrandMarkGlyph />
            </span>
            <span className="text-sm font-medium text-foreground">
              {brandLabel}
            </span>
          </div>
          <ThemeToggle />
        </header>

        <main className="relative z-10 mx-auto flex w-full max-w-sm flex-1 flex-col justify-center gap-4 py-6">
          <InsecureConnectionBanner />
          <OAuthErrorBanner />
          {sessionEnded ? (
            <Alert role="status">
              <LogOutGlassIcon size="1.4em" />
              <AlertDescription>{t('sessionEnded.notice')}</AlertDescription>
            </Alert>
          ) : null}

          <div className="space-y-2">
            <Eyebrow>
              {isRegister
                ? t('foundation.signIn.eyebrowSetup')
                : t('foundation.signIn.eyebrowSignIn')}
            </Eyebrow>
            <Display as="h1" size="greeting">
              {isRegister
                ? t('foundation.signIn.titleSetup')
                : t('foundation.signIn.titleSignIn')}
            </Display>
            <Lede>
              {isRegister
                ? t('foundation.signIn.subtitleSetup')
                : t('foundation.signIn.subtitleSignIn')}
            </Lede>
          </div>

          <div className="space-y-4">
            {showRegisterTab ? (
              <Tabs value={tab} onValueChange={handleTabChange}>
                <TabsList className="grid w-full grid-cols-2">
                  <TabsTrigger value="sign-in">
                    {t('foundation.signIn.tabSignIn')}
                  </TabsTrigger>
                  <TabsTrigger value="register">
                    {t('foundation.signIn.tabSetup')}
                  </TabsTrigger>
                </TabsList>
                <TabsContent value="sign-in" className="space-y-3 pt-2">
                  {signInContent}
                </TabsContent>
                <TabsContent value="register" className="pt-2">
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
            <DemoLoginEntry />
          </div>

          {devMode.data?.enabled ? (
            <div className="space-y-2 rounded-lg border border-dashed border-border p-3">
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
                <FlaskConicalGlassIcon size="1.4em" />
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
                To test the passkey branch: register one for {DEV_MODE_USERNAME}{' '}
                in Settings, then use Chrome DevTools &rsaquo; More tools
                &rsaquo; WebAuthn to add a virtual authenticator.
              </p>
            </div>
          ) : null}
        </main>

        {isRegister ? null : (
          <footer className="relative z-10 mx-auto w-full max-w-sm">
            <LockedOutDialog />
          </footer>
        )}
        <div className="mx-auto w-full max-w-sm">
          <LoginAttribution />
        </div>
      </div>

      <LoginBrandPanel name={brand.Name} />
    </div>
  )
}
