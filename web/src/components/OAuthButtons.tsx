import { useQuery } from '@tanstack/react-query'
import { GithubLogoIcon, GoogleLogoIcon, ShieldCheckIcon } from '@phosphor-icons/react/dist/ssr'
import type { PublicOAuthProvider } from '../queries/oauth'
import { publicOAuthProvidersQueryOptions } from '../queries/oauth'
import { Button } from './ui/button'

const PROVIDER_LABELS: Record<string, { label: string; icon: React.ReactNode }> = {
  google: { label: 'Continue with Google', icon: <GoogleLogoIcon /> },
  github: { label: 'Continue with GitHub', icon: <GithubLogoIcon /> },
}

function buttonMeta(p: PublicOAuthProvider): { label: string; icon: React.ReactNode } {
  if (p.provider === 'oidc') {
    return { label: `Continue with ${p.display_name || 'SSO'}`, icon: <ShieldCheckIcon /> }
  }
  return PROVIDER_LABELS[p.provider] ?? { label: `Continue with ${p.provider}`, icon: null }
}

// Real top-level navigation (<a href>, not a fetch): the browser must
// leave the SPA to reach the provider's own consent screen.
function startOAuthURL(provider: string): string {
  return `/api/v1/auth/oauth/${encodeURIComponent(provider)}/start`
}

// Only rendered for enabled providers; a failed query just means no
// buttons show.
export function OAuthButtons() {
  const { data } = useQuery(publicOAuthProvidersQueryOptions())
  const enabled = (data ?? []).filter((p) => p.enabled)
  if (enabled.length === 0) {
    return null
  }

  return (
    <div className="flex w-full max-w-sm flex-col gap-2">
      <div className="flex items-center gap-3 text-xs text-muted-foreground">
        <span className="h-px flex-1 bg-border" />
        or
        <span className="h-px flex-1 bg-border" />
      </div>
      {enabled.map((p) => {
        const meta = buttonMeta(p)
        return (
          <Button
            key={p.provider}
            type="button"
            variant="outline"
            render={<a href={startOAuthURL(p.provider)} />}
          >
            {meta.icon}
            {meta.label}
          </Button>
        )
      })}
    </div>
  )
}
