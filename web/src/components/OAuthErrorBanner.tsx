import { useMemo } from 'react'
import { WarningIcon } from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription } from './ui/alert'

// Maps internal/api/oauth.go's oauthErrorCode values to a message.
// Unlisted codes fall back to the generic message.
const MESSAGES: Record<string, string> = {
  provider_denied: 'Sign-in was cancelled.',
  invalid_state: 'That sign-in link expired or was already used. Try again.',
  exchange_failed: 'Sign-in with the provider failed. Try again.',
  userinfo_failed: 'Could not read your account details from the provider.',
  domain_not_allowed: 'Sign-in is restricted to a specific email domain, and your account does not match it.',
  email_in_use:
    'An account with this email already exists. Sign in with your password, then connect this provider from Settings.',
  already_linked: 'That provider account is already linked to a different user.',
}

// Reads oauth_error directly from window.location.search rather than
// through a typed route search schema: this is a one-shot read on the
// screen the OAuth callback redirects to, not app navigation state.
export function OAuthErrorBanner() {
  const code = useMemo(
    () => new URLSearchParams(window.location.search).get('oauth_error'),
    [],
  )
  if (!code) {
    return null
  }
  return (
    <Alert variant="destructive" className="w-full max-w-sm">
      <WarningIcon />
      <AlertDescription>
        {MESSAGES[code] ?? 'Sign-in failed. Try again.'}
      </AlertDescription>
    </Alert>
  )
}
