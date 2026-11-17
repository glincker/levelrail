import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { isProviderScopeMissing } from '../../lib/providerCapabilities'

// Shared by any action that can hit a 424 provider_scope_missing: names the
// scope and links to the permissions panel instead of showing a raw error.
export function ProviderScopeNotice({
  error,
  provider,
}: Readonly<{ error: unknown; provider: string }>) {
  const { t } = useTranslation('cloudProvider')
  if (!isProviderScopeMissing(error)) return null
  return (
    <p role="alert" className="text-xs text-tone-warning">
      {error.needs
        ? t('scopeNotice.needs', { scope: error.needs })
        : t('scopeNotice.needsGeneric')}{' '}
      <Link
        to="/integrations/cloud/$provider"
        params={{ provider }}
        hash="cp-capabilities"
        className="underline underline-offset-2"
      >
        {t('scopeNotice.link')}
      </Link>
    </p>
  )
}
