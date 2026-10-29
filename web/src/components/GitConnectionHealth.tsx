import { CircleCheckGlassIcon, PlugGlassIcon } from '@thesvg/ui-react'
import { useTranslation } from 'react-i18next'
import { Banner } from '@/components/ui/banner'
import { Button } from '@/components/ui/button'
import { clock } from '../lib/clockTime'
import { useGitProviders } from '../queries/gitProviders'
import type { GitProviderName, GitProviderStatus } from '../types/gitProviders'

// Where Reconnect leads: the provider's settings page, where each connected
// account has its own Reconnect button (GitHub installs the App again).
function reconnectUrl(provider: GitProviderName): string {
  return `/settings/${provider}-app`
}

// A banner with a primary Reconnect button when a connection needs the
// operator, nothing when it is healthy.
export function GitConnectionBanner({
  provider,
}: Readonly<{ provider: GitProviderStatus }>) {
  const { t } = useTranslation('gitRepos')
  const {
    status,
    detail,
    next_check_at: next,
  } = provider.connection ?? {
    status: 'ok',
  }
  if (status === 'ok') return null
  const name = t(`connection.providers.${provider.provider}`)
  if (status === 'rate_limited') {
    return (
      <Banner
        variant="warning"
        title={t('connection.rate_limited.title', { provider: name })}
      >
        {t('connection.rate_limited.body', { time: next ? clock(next) : '' })}
      </Banner>
    )
  }
  return (
    <Banner
      variant="critical"
      title={t(`connection.${status}.title`, { provider: name })}
      action={
        <Button
          type="button"
          size="sm"
          onClick={() => {
            window.location.assign(reconnectUrl(provider.provider))
          }}
        >
          <PlugGlassIcon className="size-4.5" aria-hidden="true" />
          {t('connection.reconnect', { provider: name })}
        </Button>
      }
    >
      {t(`connection.${status}.body`)}
      {detail ? ` ${detail}.` : ''}
    </Banner>
  )
}

// Connection health on a provider's settings card: the banner when
// something is wrong, a quiet confirmation otherwise.
export function GitConnectionHealth({
  provider,
}: Readonly<{ provider: GitProviderName }>) {
  const { t } = useTranslation('gitRepos')
  const { data } = useGitProviders()
  const current = data.find((p) => p.provider === provider)
  if (!current?.connected) return null
  if ((current.connection?.status ?? 'ok') === 'ok') {
    return (
      <p className="flex items-center gap-1.5 text-xs text-muted-foreground">
        <CircleCheckGlassIcon
          className="size-4 text-tone-success"
          aria-hidden="true"
        />
        {t('connection.label')}: {t('connection.healthy')}
      </p>
    )
  }
  return <GitConnectionBanner provider={current} />
}
