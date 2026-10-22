import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { Button } from '@/components/ui/button'
import { BrandTile } from '../BrandLogoBadge'
import { ingressSettingsQueryOptions } from '../../queries/domains'

const PROVIDERS = [
  { id: 'github', label: 'GitHub', to: '/settings/github-app' },
  { id: 'gitlab', label: 'GitLab', to: '/settings/gitlab-app' },
  { id: 'bitbucket', label: 'Bitbucket', to: '/settings/bitbucket-app' },
  { id: 'gitea', label: 'Gitea', to: '/settings/gitea-app' },
] as const

/** One card per supported git provider, each linking to its existing connect flow. */
export function GitProviderCards() {
  const { t } = useTranslation('newApp')
  const { data: ingress } = useQuery(ingressSettingsQueryOptions())
  const noPublicAddress = ingress !== undefined && !ingress.primary_domain
  return (
    <div className="space-y-2">
      <div>
        <p className="text-sm font-medium text-foreground">
          {t('source.git.connectTitle')}
        </p>
        <p className="text-sm text-muted-foreground">
          {t('source.git.connectBody')}
        </p>
      </div>
      <ul className="grid gap-2 sm:grid-cols-2">
        {PROVIDERS.map((p) => (
          <li
            key={p.id}
            className="flex items-center gap-3 rounded-lg border border-border bg-card p-3"
          >
            <BrandTile name={p.id} label={p.label} size="md" />
            <span className="min-w-0 flex-1 text-sm font-medium text-foreground">
              {p.label}
            </span>
            <Button
              size="sm"
              variant="outline"
              render={<Link to={p.to} />}
              nativeButton={false}
              aria-label={t('source.git.connectLabel', { provider: p.label })}
            >
              {t('source.git.connect')}
            </Button>
          </li>
        ))}
      </ul>
      {noPublicAddress ? (
        <p className="text-sm text-muted-foreground">
          {t('source.git.needsAddress')}{' '}
          <button
            type="button"
            className="font-medium text-foreground underline underline-offset-2"
            onClick={() => {
              const input = document.getElementById('new-app-url')
              input?.scrollIntoView({ block: 'center' })
              input?.focus()
            }}
          >
            {t('source.git.useUrlInstead')}
          </button>
        </p>
      ) : null}
    </div>
  )
}
