import { useTranslation } from 'react-i18next'
import { GitBranchGlassIcon } from '@thesvg/ui-react'
import { Skeleton } from '@/components/ui/skeleton'
import { PageBanner } from '../PageBanner'
import { GitConnectionsDiagram } from '../banners/GitConnectionsDiagram'
import { AddAccountMenu } from './AddAccountMenu'
import { GitAccountRow } from './GitAccountRow'
import { useGitAccounts } from '../../queries/gitAccounts'
import {
  bannerSize,
  countNeedingAttention,
  groupByHost,
} from '../../lib/gitAccounts'

export function GitConnectionsPage() {
  const { t } = useTranslation('settings')
  const { accounts, isLoading, isError } = useGitAccounts()
  const size = bannerSize(accounts)
  const attention = countNeedingAttention(accounts)
  const hero = size === 'hero'

  let body: React.ReactNode
  if (isLoading) {
    body = <Skeleton className="h-40 w-full" />
  } else if (isError) {
    body = (
      <p className="text-sm text-destructive">
        {t('gitConnectionsHub.loadFailed')}
      </p>
    )
  } else if (hero) {
    body = (
      <p className="max-w-prose text-sm text-muted-foreground">
        {t('gitConnectionsHub.banner.whatFor')}
      </p>
    )
  } else {
    body = (
      <div className="space-y-6">
        {attention > 0 ? (
          <p className="text-sm text-muted-foreground" role="status">
            {t('gitConnectionsHub.attention', { count: attention })}
          </p>
        ) : null}
        {groupByHost(accounts).map((group) => (
          <section
            key={group.provider}
            aria-labelledby={`git-host-${group.provider}`}
            className="rounded-lg border border-border bg-card p-4"
          >
            <h2
              id={`git-host-${group.provider}`}
              className="mb-3 text-sm font-semibold text-foreground"
            >
              {t(`gitConnectionsHub.hosts.${group.provider}.name`)}
            </h2>
            <ul className="divide-y divide-border">
              {group.accounts.map((account) => (
                <GitAccountRow key={account.key} account={account} />
              ))}
            </ul>
          </section>
        ))}
      </div>
    )
  }

  return (
    <div className="mx-auto flex w-full min-w-0 max-w-(--glinr-content-max) flex-col gap-(--glinr-page-gap)">
      <PageBanner
        icon={<GitBranchGlassIcon size="1.4em" />}
        titleKey="settings:gitConnectionsHub.title"
        descriptionKey={
          hero
            ? 'settings:gitConnectionsHub.banner.description'
            : 'settings:gitConnectionsHub.description'
        }
        action={<AddAccountMenu />}
        illustration={<GitConnectionsDiagram size={size} />}
        size={size}
      />
      {body}
    </div>
  )
}
