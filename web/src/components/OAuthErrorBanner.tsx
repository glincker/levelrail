import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from './ui/alert'

// Short codes from internal/api/oauth.go and sso_saml_flow.go; others fall back to generic.
const KNOWN_CODES = [
  'provider_denied',
  'invalid_state',
  'exchange_failed',
  'userinfo_failed',
  'domain_not_allowed',
  'email_in_use',
  'already_linked',
  'rate_limited',
  'saml_unavailable',
  'saml_invalid',
  'saml_replayed',
  'sso_not_provisioned',
  'sso_no_role',
  'sso_no_email',
  'account_disabled',
  'sso_root_not_linkable',
] as const

type KnownCode = (typeof KNOWN_CODES)[number]

function isKnownCode(code: string): code is KnownCode {
  return (KNOWN_CODES as readonly string[]).includes(code)
}

// One-shot read on the screen the OAuth and SAML callbacks redirect to, not app navigation state.
export function OAuthErrorBanner() {
  const { t } = useTranslation('common')
  const code = useMemo(
    () => new URLSearchParams(window.location.search).get('oauth_error'),
    [],
  )
  if (!code) {
    return null
  }
  return (
    <Alert variant="destructive" className="w-full max-w-sm">
      <TriangleAlertGlassIcon className="text-tone-warning" size="1.7em" />
      <AlertDescription>
        {isKnownCode(code)
          ? t(`foundation.signIn.errors.${code}`)
          : t('foundation.signIn.errors.generic')}
      </AlertDescription>
    </Alert>
  )
}
