import { useTranslation } from 'react-i18next'
import {
  PageRail,
  PageRailList,
  PageRailRow,
  PageRailSection,
} from '@/components/shell/PageRail'
import { Badge } from '@/components/ui/badge'
import { HelpLink } from '../HelpLink'
import {
  useGitHubAppInstallations,
  useGitHubAppStatus,
} from '../../queries/githubApp'
import { useGitLabAppStatus } from '../../queries/gitlabApp'
import { useGiteaAppStatus } from '../../queries/giteaApp'
import { useBitbucketAppStatus } from '../../queries/bitbucketApp'

export type RailProvider = 'github' | 'gitlab' | 'gitea' | 'bitbucket'

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
  const access = t(`gitProviders.rail.access.${provider}`, {
    returnObjects: true,
  })

  return (
    <PageRail>
      <PageRailSection title={t('gitProviders.rail.connection')}>
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

function GitLabRail() {
  const { data: status } = useGitLabAppStatus()
  return (
    <GitProviderRailBody
      provider="gitlab"
      connected={status.connected && status.authorized}
      instance={status.instance_url}
    />
  )
}

function GiteaRail() {
  const { data: status } = useGiteaAppStatus()
  return (
    <GitProviderRailBody
      provider="gitea"
      connected={status.connected && status.authorized}
      instance={status.instance_url}
    />
  )
}

function BitbucketRail() {
  const { data: status } = useBitbucketAppStatus()
  return (
    <GitProviderRailBody
      provider="bitbucket"
      connected={status.connected && status.authorized}
      instance="https://bitbucket.org"
    />
  )
}

/** GitProviderRail is the right rail of the four git provider pages, built from the status each page already loads. */
export function GitProviderRail({ provider }: { provider: RailProvider }) {
  switch (provider) {
    case 'github':
      return <GitHubRail />
    case 'gitlab':
      return <GitLabRail />
    case 'gitea':
      return <GiteaRail />
    case 'bitbucket':
      return <BitbucketRail />
  }
}
