import { createFileRoute } from '@tanstack/react-router'
import { lazy, Suspense, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { KeyGlassIcon } from '@thesvg/ui-react'
import {
  rootTokensQueryOptions,
  tokenCountsQueryOptions,
} from '../../queries/tokenList'
import { CreateTokenDialog } from '../../components/CreateTokenDialog'
import { useBrand } from '../../hooks/useBrand'
import { TableSkeleton } from '@/components/ui/table-skeleton'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { Badge } from '@/components/ui/badge'
import { useTokenExpiryPolicy } from '../../hooks/useTokenExpiryPolicy'
import { TokensNotices } from '../../components/tokens/TokensNotices'
import { useTranslation } from 'react-i18next'
import { SettingsPageTitle } from '../../components/settings/SettingsPageTitle'
import type { StatusFilter } from '../../components/tokens/TokensToolbar'

const TokensExplorer = lazy(() =>
  import('../../components/tokens/TokensExplorer').then((m) => ({
    default: m.TokensExplorer,
  })),
)

// Account-level, so it lives under routes/settings/. The loader primes
// the cache; the component only reads it.
export const Route = createFileRoute('/settings/tokens')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(tokenCountsQueryOptions({})),
  component: TokensPage,
  pendingComponent: TokensPending,
})

function TokensPage() {
  const { t } = useTranslation('settings')
  const brand = useBrand()
  const policy = useTokenExpiryPolicy()
  const { t: te } = useTranslation('tokenExpiry')
  const [status, setStatus] = useState<StatusFilter>('active')
  const counts = useQuery(tokenCountsQueryOptions({})).data?.counts
  const longLivedRoot = useQuery(rootTokensQueryOptions()).data ?? 0
  const active = counts?.active ?? 0

  return (
    <PageTemplate
      icon={<KeyGlassIcon size="1.4em" />}
      eyebrow={t('tokensPage.eyebrow')}
      title={t('tokensPage.title')}
      description={t('tokensPage.description')}
      helpPath="/api-reference"
      helpLabel={t('tokensPage.help')}
      status={
        <Badge variant={active > 0 ? 'success' : 'muted'}>
          {t('tokensPage.activeCount', { count: active })}
        </Badge>
      }
      actions={<CreateTokenDialog />}
      banners={
        <TokensNotices
          expiringSoon={counts?.expiring ?? 0}
          soonDays={policy.soonDays}
          longLivedRoot={longLivedRoot}
          onReviewExpiring={() => {
            setStatus('expiring')
          }}
        />
      }
    >
      {brand.DocsURL ? (
        <a
          href={brand.DocsURL}
          target="_blank"
          rel="noreferrer"
          className="inline-block text-sm text-primary underline underline-offset-4 hover:no-underline"
        >
          {t('tokensPage.docsLink')}
        </a>
      ) : null}
      {policy.windows && policy.grace ? (
        <p className="text-xs text-muted-foreground">
          {te('policy', { windows: policy.windows, grace: policy.grace })}
        </p>
      ) : null}
      <Suspense fallback={<TableSkeleton columnCount={6} />}>
        <TokensExplorer status={status} onStatus={setStatus} />
      </Suspense>
    </PageTemplate>
  )
}

// Route-level fallback for the loader's pending phase.
function TokensPending() {
  return (
    <div className="space-y-6">
      <SettingsPageTitle id="tokens" />
      <TableSkeleton columnCount={6} />
    </div>
  )
}
