import { useEffect, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  FlaskIcon,
  SignOutIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import { devModeQueryOptions } from '../queries/devMode'
import { useBrand } from '../hooks/useBrand'
import {
  setupStatusQueryOptions,
  useConsumeSessionLink,
  useLogin,
} from '../queries/auth'
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
import { Alert, AlertDescription } from './ui/alert'
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
// installer's ?setup=<token> link was opened. sessionLink, when present
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
        <AuthBrandMark label={brandLabel} />
        <Card className="w-full max-w-sm shadow-sm">
          <CardContent className="space-y-4 pt-6">
            {consumeSessionLink.isError ? (
              <>
                <Alert variant="destructive">
                  <WarningIcon />
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
    <div className="relative flex min-h-[70vh] flex-col items-center justify-center gap-6 px-4 py-10">
      <div className="absolute top-4 right-4">
        <ThemeToggle />
      </div>
      <AuthBrandMark label={brandLabel} />
      <div className="w-full max-w-sm">
        <InsecureConnectionBanner />
      </div>
      <OAuthErrorBanner />
      {sessionEnded ? (
        <Alert
          className="w-full max-w-sm border-tone-info-border bg-tone-info-soft text-tone-info"
          role="status"
        >
          <SignOutIcon />
          <AlertDescription>{t('sessionEnded.notice')}</AlertDescription>
        </Alert>
      ) : null}
      <Card className="w-full max-w-sm shadow-sm">
        <CardHeader>
          <CardTitle role="heading" aria-level={1}>
            {isRegister ? t('login.setupTitle') : t('login.signInTitle')}
          </CardTitle>
          <CardDescription>
            {isRegister ? t('login.setupHint') : t('login.signInHint')}
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {showRegisterTab ? (
            <Tabs value={tab} onValueChange={handleTabChange}>
              <TabsList className="grid w-full grid-cols-2">
                <TabsTrigger value="sign-in">
                  {t('login.signInTab')}
                </TabsTrigger>
                <TabsTrigger value="register">
                  {t('login.setupTab')}
                </TabsTrigger>
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
      {isRegister ? null : (
        <p className="max-w-sm text-center text-xs text-muted-foreground">
          {t('loginHelp.lockedOut')}{' '}
          <code className="rounded bg-muted px-1 py-0.5 font-mono">
            {brand.BinaryName ? `sudo ${brand.BinaryName} ` : ''}recover-admin
          </code>
          {brand.DocsURL ? (
            <>
              {' '}
              <a
                href={`${brand.DocsURL}/emergency-access`}
                target="_blank"
                rel="noreferrer"
                className="underline"
              >
                {t('loginHelp.emergencyDocs')}
              </a>
            </>
          ) : null}
        </p>
      )}
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

function AuthBrandMark({ label }: { label: string }) {
  return (
    <div className="flex flex-col items-center gap-2 text-center">
      <div
        aria-hidden="true"
        className="flex size-11 items-center justify-center rounded-lg border border-border bg-card p-2 text-base font-semibold text-foreground"
      >
        <BrandMarkGlyph />
      </div>
      <span className="text-sm font-medium text-foreground">{label}</span>
    </div>
  )
}
