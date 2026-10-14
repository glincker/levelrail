import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { BuildingsIcon } from '@phosphor-icons/react/dist/ssr'
import type { PublicOAuthProvider } from '../queries/oauth'
import { publicOAuthProvidersQueryOptions } from '../queries/oauth'
import { usePublicSAMLProviders } from '../queries/sso'
import { brandKeyFor } from '../lib/oauthProviders'
import { BrandTile } from './BrandLogoBadge'
import { Button } from './ui/button'

const PROVIDER_NAMES: Record<string, string> = {
  google: 'Google',
  github: 'GitHub',
  microsoft: 'Microsoft',
  gitlab: 'GitLab',
  bitbucket: 'Bitbucket',
  discord: 'Discord',
  slack: 'Slack',
  linkedin: 'LinkedIn',
  twitch: 'Twitch',
  facebook: 'Facebook',
}

// Real top-level navigation (<a href>, not a fetch): the browser must
// leave the SPA to reach the provider's own consent screen.
function startOAuthURL(provider: string): string {
  return `/api/v1/auth/oauth/${encodeURIComponent(provider)}/start`
}

function startSAMLURL(id: string): string {
  return `/api/v1/auth/saml/${encodeURIComponent(id)}/login`
}

// Only rendered for enabled providers; a failed query just means no
// buttons show.
export function OAuthButtons() {
  const { t } = useTranslation('common')
  const { data } = useQuery(publicOAuthProvidersQueryOptions())
  const saml = usePublicSAMLProviders()
  const enabled = (data ?? []).filter((p) => p.enabled)
  const samlProviders = saml.data ?? []
  if (enabled.length === 0 && samlProviders.length === 0) {
    return null
  }

  const nameFor = (p: PublicOAuthProvider): string =>
    p.provider === 'oidc'
      ? p.display_name || t('foundation.signIn.sso')
      : (PROVIDER_NAMES[p.provider] ?? p.provider)

  return (
    <div className="flex w-full max-w-sm flex-col gap-2">
      <div className="flex items-center gap-3 text-xs text-muted-foreground">
        <span className="h-px flex-1 bg-border" />
        {t('foundation.signIn.or')}
        <span className="h-px flex-1 bg-border" />
      </div>
      {samlProviders.map((p) => (
        <Button
          key={p.id}
          type="button"
          variant="outline"
          size="lg"
          className="justify-start gap-3"
          render={<a href={startSAMLURL(p.id)} />}
        >
          <span className="flex size-6 items-center justify-center rounded bg-muted text-muted-foreground">
            <BuildingsIcon aria-hidden="true" />
          </span>
          {t('foundation.signIn.continueWith', { provider: p.name })}
        </Button>
      ))}
      {enabled.map((p) => {
        const name = nameFor(p)
        return (
          <Button
            key={p.provider}
            type="button"
            variant="outline"
            size="lg"
            className="justify-start gap-3"
            render={<a href={startOAuthURL(p.provider)} />}
          >
            <BrandTile
              name={brandKeyFor(p.provider)}
              label={name}
              size="sm"
              className="size-6 rounded"
            />
            {t('foundation.signIn.continueWith', { provider: name })}
          </Button>
        )
      })}
    </div>
  )
}
