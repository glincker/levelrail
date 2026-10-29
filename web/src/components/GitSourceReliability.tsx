import { RefreshCwGlassIcon, TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { useIsRoot } from '../hooks/useIsRoot'
import { useRegisterGitSourceWebhook } from '../queries/gitSources'
import { useFixForCheck } from '../queries/fixit'
import type { GitSourceResource } from '../types/gitSource'
import { FixAction } from './fixit/FixAction'

function when(iso?: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? '' : d.toLocaleString()
}

type ConnectableProvider = 'gitlab' | 'bitbucket' | 'gitea'

// The provider pages that can reconnect a source whose account is gone.
// Without a known provider the repository host is the only hint.
function reconnectProviders(source: GitSourceResource): ConnectableProvider[] {
  if (source.connection?.provider) return [source.connection.provider]
  const host = new URL(source.repo_url, 'https://x.invalid').hostname
  if (host === 'bitbucket.org') return ['bitbucket']
  if (host.includes('gitlab')) return ['gitlab']
  if (/gitea|forgejo|codeberg/.test(host)) return ['gitea']
  return ['gitlab', 'gitea']
}

// What a missed webhook looks like from the source's side: an unreachable
// webhook address with a Fix, a registration that failed with Register again,
// and when the poller and webhooks were last heard from.
export function GitSourceReliability({
  appName,
  source,
}: Readonly<{ appName: string; source: GitSourceResource }>) {
  const { t } = useTranslation('gitRepos')
  const isRoot = useIsRoot()
  const register = useRegisterGitSourceWebhook(appName)
  const urlFix = useFixForCheck('webhook_public_url')
  const reg = source.webhook_registration
  const poll = source.poll

  return (
    <div className="space-y-2" data-testid="git-source-reliability">
      {source.connection &&
      !['ok', 'rate_limited'].includes(source.connection.state) ? (
        <Alert variant="destructive">
          <TriangleAlertGlassIcon
            className="size-5 text-tone-warning"
            aria-hidden="true"
          />
          <AlertDescription className="space-y-2">
            <p>
              {t(`reliability.connection.${source.connection.state}`, {
                label: source.connection.label ?? '',
                defaultValue: t('reliability.connection.needs_reauthorize', {
                  label: source.connection.label ?? '',
                }),
              })}
            </p>
            <div className="flex flex-wrap gap-2">
              {reconnectProviders(source).map((provider) => (
                <Button
                  key={provider}
                  size="sm"
                  render={<a href={`/settings/${provider}-app`} />}
                  nativeButton={false}
                >
                  {t('reliability.connection.reconnect', {
                    provider: t(`connection.providers.${provider}`),
                  })}
                </Button>
              ))}
            </div>
          </AlertDescription>
        </Alert>
      ) : null}
      {source.webhook_url_warning ? (
        <Alert variant="destructive">
          <TriangleAlertGlassIcon
            className="size-5 text-tone-warning"
            aria-hidden="true"
          />
          <AlertDescription className="space-y-2">
            <p>{t(`reliability.urlWarning.${source.webhook_url_warning}`)}</p>
            {urlFix && isRoot ? (
              <FixAction fix={urlFix} />
            ) : (
              <p>{t('reliability.urlFixHint')}</p>
            )}
          </AlertDescription>
        </Alert>
      ) : null}
      {reg?.state === 'failed' ? (
        <Alert variant="destructive">
          <TriangleAlertGlassIcon
            className="size-5 text-tone-warning"
            aria-hidden="true"
          />
          <AlertDescription className="space-y-2">
            <p>
              {t('reliability.registration.failed', { error: reg.error ?? '' })}
            </p>
            <Button
              type="button"
              size="sm"
              disabled={register.isPending}
              onClick={() => {
                register.mutate(undefined, {
                  onSuccess: (r) => {
                    if (r.webhook_registration?.state === 'registered') {
                      toast.add({
                        title: t('reliability.registration.done'),
                        type: 'success',
                      })
                    }
                  },
                  onError: (e) => {
                    toast.add({
                      title: t('reliability.registration.failedToast'),
                      description: e.message,
                      type: 'error',
                    })
                  },
                })
              }}
            >
              <RefreshCwGlassIcon className="size-5" aria-hidden="true" />
              {register.isPending
                ? t('reliability.registration.registering')
                : t('reliability.registration.registerAgain')}
            </Button>
          </AlertDescription>
        </Alert>
      ) : null}
      {reg?.state === 'registered' ? (
        <p className="text-xs text-muted-foreground">
          {t('reliability.registration.registered')}
        </p>
      ) : null}
      {poll ? (
        <p className="text-xs text-muted-foreground">
          {poll.enabled
            ? poll.last_polled_at
              ? t('reliability.poll.last', { time: when(poll.last_polled_at) })
              : t('reliability.poll.never')
            : t('reliability.poll.disabled')}{' '}
          {poll.last_webhook_at
            ? t('reliability.poll.lastWebhook', {
                time: when(poll.last_webhook_at),
              })
            : t('reliability.poll.noWebhook')}
          {poll.error
            ? ` ${t('reliability.poll.error', { error: poll.error })}`
            : ''}
        </p>
      ) : null}
    </div>
  )
}
