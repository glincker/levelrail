import { useQueries, useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  BracesGlassIcon,
  PlugGlassIcon,
  ShieldAlertGlassIcon,
  ShieldCheckGlassIcon,
} from '@thesvg/ui-react'
import { buttonVariants } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { StatusBadge } from '@/components/ui/status-badge'
import { BrandTile } from '@/components/BrandLogoBadge'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { organizationEnvQueryOptions } from '@/queries/organizationEnv'
import { organizationListQueryOptions } from '@/queries/organizations'
import { projectEnvQueryOptions } from '@/queries/projectEnv'
import { projectListQueryOptions } from '@/queries/projects'
import { gitProvidersQueryOptions } from '@/queries/gitProviders'
import type { GitProviderName } from '@/types/gitProviders'
import type { Tone } from '@/components/kit/tone'

const LINK = buttonVariants({ variant: 'outline', size: 'sm' })
const ROW =
  'flex items-center justify-between gap-3 rounded-md border border-border px-3 py-2'

// Only key counts are read; values from the env endpoints are never rendered.
function countKeys(data: Record<string, string> | undefined): number | null {
  return data ? Object.keys(data).length : null
}

export function VariablesHub() {
  const { t } = useTranslation('hubs', { keyPrefix: 'variables' })
  const orgs = useQuery(organizationListQueryOptions())
  const projects = useQuery(projectListQueryOptions())
  const orgList = orgs.data ?? []
  const projectList = projects.data ?? []
  const orgEnvs = useQueries({
    queries: orgList.map((o) => organizationEnvQueryOptions(o.id)),
  })
  const projectEnvs = useQueries({
    queries: projectList.map((p) => projectEnvQueryOptions(p.id)),
  })
  const count = (n: number | null) =>
    n === null ? t('count.unknown') : t('count.vars', { count: n })
  return (
    <PageTemplate title={t('page.title')} description={t('page.description')}>
      <div className="space-y-6">
        <section aria-labelledby="vars-org" className="space-y-2">
          <h2 id="vars-org" className="text-sm font-medium">
            {t('org.title')}
          </h2>
          {orgList.length === 0 ? (
            <EmptyState
              compact
              icon={<BracesGlassIcon className="size-6" />}
              title={t('org.empty.title')}
              description={t('org.empty.description')}
            />
          ) : (
            <ul className="space-y-2">
              {orgList.map((o, i) => (
                <li key={o.id} className={ROW}>
                  <span className="text-sm">{o.name}</span>
                  <span className="flex items-center gap-3">
                    <span className="text-sm text-muted-foreground">
                      {count(countKeys(orgEnvs[i]?.data))}
                    </span>
                    <Link
                      to="/organizations/$id"
                      params={{ id: o.id }}
                      className={LINK}
                    >
                      {t('open')}
                    </Link>
                  </span>
                </li>
              ))}
            </ul>
          )}
        </section>
        <section aria-labelledby="vars-project" className="space-y-2">
          <h2 id="vars-project" className="text-sm font-medium">
            {t('project.title')}
          </h2>
          {projectList.length === 0 ? (
            <EmptyState
              compact
              icon={<BracesGlassIcon className="size-6" />}
              title={t('project.empty.title')}
              description={t('project.empty.description')}
            />
          ) : (
            <ul className="space-y-2">
              {projectList.map((p, i) => (
                <li key={p.id} className={ROW}>
                  <span className="text-sm">{p.name}</span>
                  <span className="flex items-center gap-3">
                    <span className="text-sm text-muted-foreground">
                      {count(countKeys(projectEnvs[i]?.data))}
                    </span>
                    <Link
                      to="/projects/$id"
                      params={{ id: p.id }}
                      className={LINK}
                    >
                      {t('open')}
                    </Link>
                  </span>
                </li>
              ))}
            </ul>
          )}
          <p className="text-sm text-muted-foreground">
            {t('project.environments')}
          </p>
        </section>
        <section aria-labelledby="vars-apps" className="space-y-2">
          <h2 id="vars-apps" className="text-sm font-medium">
            {t('apps.title')}
          </h2>
          <div className={ROW}>
            <span className="text-sm text-muted-foreground">
              {t('apps.note')}
            </span>
            <Link to="/apps" className={LINK}>
              {t('apps.open')}
            </Link>
          </div>
        </section>
      </div>
    </PageTemplate>
  )
}

type ProviderTo =
  | '/settings/github-app'
  | '/settings/gitlab-app'
  | '/settings/bitbucket-app'
  | '/settings/bitbucket-server-app'
  | '/settings/gitea-app'
  | '/settings/forgejo-app'
  | '/settings/azure-devops-app'

const PROVIDER_TO: Record<GitProviderName, ProviderTo> = {
  github: '/settings/github-app',
  gitlab: '/settings/gitlab-app',
  bitbucket: '/settings/bitbucket-app',
  bitbucket_server: '/settings/bitbucket-server-app',
  gitea: '/settings/gitea-app',
  forgejo: '/settings/forgejo-app',
  azure_devops: '/settings/azure-devops-app',
}

function healthTone(status: string | undefined): Tone {
  if (status === 'revoked') return 'danger'
  if (status === 'needs_reauthorize' || status === 'rate_limited')
    return 'warning'
  return 'success'
}

// Brand marks come from the thesvg library through BrandTile, never vendored.
const GIT_PROVIDER_BRAND: Record<GitProviderName, string> = {
  github: 'github',
  gitlab: 'gitlab',
  bitbucket: 'bitbucket',
  bitbucket_server: 'bitbucket',
  gitea: 'gitea',
  forgejo: 'forgejo',
  azure_devops: 'azure',
}

export function SourcesHub() {
  const { t } = useTranslation('hubs', { keyPrefix: 'sources' })
  const providers = useQuery(gitProvidersQueryOptions())
  return (
    <PageTemplate title={t('page.title')} description={t('page.description')}>
      {providers.isPending ? (
        <p className="text-sm text-muted-foreground">{t('loading')}</p>
      ) : providers.isError ? (
        <p role="alert" className="text-sm text-destructive">
          {t('error')}
        </p>
      ) : providers.data.length === 0 ? (
        <EmptyState
          icon={<PlugGlassIcon className="size-6" />}
          title={t('empty.title')}
          description={t('empty.description')}
        />
      ) : (
        <ul className="space-y-2">
          {providers.data.map((p) => {
            const status = p.connection?.status
            const tone: Tone = p.connected ? healthTone(status) : 'neutral'
            const label = !p.connected
              ? t('status.notConnected')
              : t(`status.${status ?? 'ok'}`)
            return (
              <li key={p.provider} className={ROW}>
                <span className="flex items-center gap-2.5 text-sm font-medium">
                  <BrandTile
                    name={GIT_PROVIDER_BRAND[p.provider]}
                    label={t(`provider.${p.provider}`)}
                    size="sm"
                  />
                  {t(`provider.${p.provider}`)}
                </span>
                <span className="flex items-center gap-3">
                  <StatusBadge
                    tone={tone}
                    label={label}
                    icon={
                      tone === 'success'
                        ? ShieldCheckGlassIcon
                        : tone === 'neutral'
                          ? PlugGlassIcon
                          : ShieldAlertGlassIcon
                    }
                  />
                  <span className="text-sm text-muted-foreground">
                    {t('accounts', { count: p.connections ?? 0 })}
                  </span>
                  <Link to={PROVIDER_TO[p.provider]} className={LINK}>
                    {t('manage')}
                  </Link>
                </span>
              </li>
            )
          })}
        </ul>
      )}
    </PageTemplate>
  )
}
