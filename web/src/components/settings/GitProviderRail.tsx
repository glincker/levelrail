import { useTranslation } from 'react-i18next'
import {
  PageRail,
  PageRailList,
  PageRailRow,
  PageRailSection,
} from '@/components/shell/PageRail'
import { Badge } from '@/components/ui/badge'
import { BrandTile } from '@/components/BrandLogoBadge'
import { HelpLink } from '../HelpLink'
import {
  useGitHubAppInstallations,
  useGitHubAppStatus,
} from '../../queries/githubApp'
import { useGitConnections } from '../../queries/gitConnections'

// Brand marks come from the thesvg library through BrandTile, never vendored.
const RAIL_BRAND: Record<RailProvider, string> = {
  github: 'github',
  gitlab: 'gitlab',
  gitea: 'gitea',
  forgejo: 'forgejo',
  bitbucket: 'bitbucket',
  bitbucket_server: 'bitbucket',
  azure_devops: 'azure',
}

export type RailProvider =
  | 'github'
  | 'gitlab'
  | 'gitea'
  | 'forgejo'
  | 'bitbucket'
  | 'bitbucket_server'
  | 'azure_devops'

interface RailBodyProps {
  provider: RailProvider
  connected: boolean
  account?: string
  accounts?: number
  instance?: string
}

function GitProviderRailBody({
  provider,
  connected,
  account,
  accounts,
  instance,
}: RailBodyProps) {
  const { t } = useTranslation('settings')
  const { t: th } = useTranslation('hubs')
  const providerName = th(`sources.provider.${provider}`)
  const access = t(`gitProviders.rail.access.${provider}`, {
    returnObjects: true,
  })

  return (
    <PageRail>
      <PageRailSection title={t('gitProviders.rail.connection')}>
        <PageRailRow label={t('gitProviders.eyebrow')}>
          <span className="flex items-center gap-2">
            <BrandTile
              name={RAIL_BRAND[provider]}
              label={providerName}
              size="sm"
            />
            {providerName}
          </span>
        </PageRailRow>
        <PageRailRow label={t('gitProviders.rail.status')}>
          <Badge variant={connected ? 'success' : 'muted'}>
            {connected
              ? t('gitProviders.rail.connected')
              : t('gitProviders.rail.notConnected')}
          </Badge>
        </PageRailRow>
        {account ? (
          <PageRailRow label={t('gitProviders.rail.account')}>
            {account}
          </PageRailRow>
        ) : null}
        {accounts === undefined ? null : (
          <PageRailRow label={t('gitProviders.rail.accounts')}>
            {accounts}
          </PageRailRow>
        )}
        {instance ? (
          <PageRailRow label={t('gitProviders.rail.instance')}>
            <span className="font-mono text-xs">{instance}</span>
          </PageRailRow>
        ) : null}
      </PageRailSection>
      <PageRailSection title={t('gitProviders.rail.accessTitle')}>
        <PageRailList items={access} />
      </PageRailSection>
      <PageRailSection title={t('gitProviders.rail.docs')}>
        <HelpLink
          path="/git-integrations"
          label={t('gitProviders.rail.docsLink')}
          variant="inline"
        />
      </PageRailSection>
    </PageRail>
  )
}

function GitHubRail() {
  const { data: status } = useGitHubAppStatus()
  const installs = useGitHubAppInstallations(status.connected)
  return (
    <GitProviderRailBody
      provider="github"
      connected={status.connected}
      account={status.account_login}
      accounts={installs.data?.installations.length}
      instance={status.instance_url}
    />
  )
}

function ConnectionsRail({
  provider,
}: Readonly<{ provider: Exclude<RailProvider, 'github'> }>) {
  const { data: connections } = useGitConnections(provider)
  const authorized = connections.filter((c) => c.authorized)
  const first = authorized.find((c) => c.is_default) ?? authorized[0]
  return (
    <GitProviderRailBody
      provider={provider}
      connected={authorized.length > 0}
      account={authorized.length === 1 ? first?.account_name : undefined}
      accounts={authorized.length}
      instance={first?.instance_url}
    />
  )
}

/** GitProviderRail is the right rail of the four git provider pages, built from the status each page already loads. */
export function GitProviderRail({ provider }: { provider: RailProvider }) {
  if (provider === 'github') return <GitHubRail />
  return <ConnectionsRail provider={provider} />
}
