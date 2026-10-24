import { createFileRoute } from '@tanstack/react-router'
import { useState } from 'react'
import { useSuspenseQuery } from '@tanstack/react-query'
import { KeyIcon } from '@phosphor-icons/react/dist/ssr'
import { tokenListQueryOptions } from '../../queries/tokens'
import { TokenTable } from '../../components/TokenTable'
import { CreateTokenDialog } from '../../components/CreateTokenDialog'
import { RevokedTokensCard } from '../../components/RevokedTokensCard'
import { useBrand } from '../../hooks/useBrand'
import { TableSkeleton } from '@/components/ui/table-skeleton'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { Badge } from '@/components/ui/badge'
import { Banner } from '@/components/ui/banner'
import { useTranslation } from 'react-i18next'
import { SettingsPageTitle } from '../../components/settings/SettingsPageTitle'

const EXPIRING_SOON_MS = 14 * 24 * 60 * 60 * 1000

// Account-level, so it lives under routes/settings/. The loader primes
// the cache; the component only reads it.
export const Route = createFileRoute('/settings/tokens')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(tokenListQueryOptions()),
  component: TokensPage,
  pendingComponent: TokensPending,
})

function TokensPage() {
  const { t } = useTranslation('settings')
  const { data: tokens } = useSuspenseQuery(tokenListQueryOptions())
  const brand = useBrand()
  const active = tokens.filter((tok) => !tok.revoked_at)
  const longLivedRoot = active.filter(
    (tok) => tok.abilities.includes('root') && !tok.expires_at,
  ).length
  const [soonCutoff] = useState(() => Date.now() + EXPIRING_SOON_MS)
  const expiringSoon = active.filter(
    (tok) => tok.expires_at && Date.parse(tok.expires_at) < soonCutoff,
  ).length

  return (
    <PageTemplate
      icon={<KeyIcon />}
      eyebrow={t('tokensPage.eyebrow')}
      title={t('tokensPage.title')}
      description={t('tokensPage.description')}
      helpPath="/api-reference"
      helpLabel={t('tokensPage.help')}
      status={
        <Badge variant={active.length > 0 ? 'success' : 'muted'}>
          {t('tokensPage.activeCount', { count: active.length })}
        </Badge>
      }
      actions={<CreateTokenDialog />}
      banners={
        <>
          {expiringSoon > 0 ? (
            <Banner
              variant="warning"
              title={t('tokensPage.expiring.title', { count: expiringSoon })}
              action={<CreateTokenDialog />}
            >
              {t('tokensPage.expiring.body')}
            </Banner>
          ) : null}
          {longLivedRoot > 0 ? (
            <Banner
              variant="warning"
              title={t('tokensPage.banner.title', { count: longLivedRoot })}
            >
              {t('tokensPage.banner.body')}
            </Banner>
          ) : null}
        </>
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
      <TokenTable tokens={active} />
      <RevokedTokensCard tokens={tokens} />
    </PageTemplate>
  )
}

// Route-level fallback for the loader's pending phase, matching
// TokenTable's own 8-column shape so the skeleton doesn't jump when real
// rows swap in.
function TokensPending() {
  return (
    <div className="space-y-6">
      <SettingsPageTitle id="tokens" />
      <TableSkeleton columnCount={8} />
    </div>
  )
}
