import { useEffect, useMemo, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import { loginOptionsQueryOptions } from '../queries/signIn'
import { getStoredUsername } from '../lib/authStore'
import { Alert, AlertDescription } from './ui/alert'
import { Button } from './ui/button'
import { PROVIDER_NAMES } from '../lib/oauthProviders'
import {
  SSO_REDIRECT_DELAY_MS,
  markAttempt,
  recentlyAttempted,
} from '../lib/ssoAutoRedirect'

// Starts the single configured SSO provider on the login route, never trapping
// the user: password mode, a failed or ended attempt and a recent redirect all skip it.
export function SsoAutoRedirect({
  passwordMode,
  sessionEnded,
  navigate = (url) => window.location.assign(url),
}: {
  passwordMode: boolean
  sessionEnded: boolean
  navigate?: (url: string) => void
}) {
  const { t } = useTranslation('common')
  const options = useQuery(loginOptionsQueryOptions())
  const failed = useMemo(
    () => new URLSearchParams(window.location.search).has('oauth_error'),
    [],
  )
  const [cancelled, setCancelled] = useState(false)
  const [skipGuard] = useState(recentlyAttempted)
  const provider = options.data?.sso_provider ?? ''
  const enabled = options.data?.sso_auto_redirect === true && provider !== ''
  const blocked =
    passwordMode || failed || sessionEnded || getStoredUsername() !== null
  const active = enabled && !blocked && !skipGuard && !cancelled
  const started = useRef(false)

  useEffect(() => {
    if (!active || started.current) {
      return
    }
    const timer = window.setTimeout(() => {
      started.current = true
      markAttempt()
      navigate(`/api/v1/auth/oauth/${encodeURIComponent(provider)}/start`)
    }, SSO_REDIRECT_DELAY_MS)
    return () => window.clearTimeout(timer)
  }, [active, provider, navigate])

  const name = PROVIDER_NAMES[provider] ?? provider
  if (active) {
    return (
      <Alert role="status" aria-live="polite">
        <AlertDescription className="flex items-center justify-between gap-3">
          <span>{t('ssoAutoRedirect.redirecting', { provider: name })}</span>
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() => setCancelled(true)}
          >
            {t('ssoAutoRedirect.cancel')}
          </Button>
        </AlertDescription>
      </Alert>
    )
  }
  if (enabled && !passwordMode && (failed || sessionEnded || cancelled)) {
    return (
      <div className="text-sm" aria-live="polite">
        {cancelled ? (
          <p className="sr-only">{t('ssoAutoRedirect.cancelled')}</p>
        ) : null}
        <Link
          to="/login"
          search={{ password: '1' }}
          className="text-muted-foreground underline hover:text-foreground"
        >
          {t('ssoAutoRedirect.passwordInstead')}
        </Link>
      </div>
    )
  }
  return null
}
