import { useTranslation } from 'react-i18next'
import { Banner } from '@/components/ui/banner'
import { clock } from '../lib/clockTime'
import type { GitRepoListResponse } from '../types/gitRepos'

// Says how complete and fresh the list is, so a capped, saved or rate
// limited list is never mistaken for the whole account.
export function GitRepoListStatus({
  page,
  shown,
}: Readonly<{ page: GitRepoListResponse; shown: number }>) {
  const { t } = useTranslation('gitRepos')
  const narrowing = page.total > shown
  return (
    <div className="space-y-1.5" data-testid="git-repo-list-status">
      <p className="text-xs text-muted-foreground">
        {t('picker.status.showing', { shown, total: page.total })}
        {narrowing ? ` ${t('picker.status.searchToNarrow')}` : ''}
        {page.loaded > page.total
          ? ` ${t('picker.status.loadedOf', { loaded: page.loaded })}`
          : ''}
        {page.cached_at
          ? ` ${t('picker.status.updated', { time: clock(page.cached_at) })}`
          : ''}
        {page.stale ? ` ${t('picker.status.stale')}` : ''}
      </p>
      {page.truncated ? (
        <Banner
          variant="warning"
          title={t('picker.status.truncated', { cap: page.cap })}
        />
      ) : null}
      {page.rate_limit ? (
        <Banner
          variant="warning"
          title={t('picker.status.rateLimited', {
            time: clock(page.rate_limit.next_check_at),
          })}
        />
      ) : null}
      {(page.errors ?? []).map((e) => (
        <Banner
          key={e.connection_id || e.account_login}
          variant="warning"
          title={t('picker.accountError', {
            account: e.connection_label || e.account_login,
            error: e.error,
          })}
        />
      ))}
    </div>
  )
}
