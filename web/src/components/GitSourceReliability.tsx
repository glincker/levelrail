import {
  ArrowsClockwiseIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
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
      {source.webhook_url_warning ? (
        <Alert variant="destructive">
          <WarningIcon className="size-4" aria-hidden="true" />
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
          <WarningIcon className="size-4" aria-hidden="true" />
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
              <ArrowsClockwiseIcon className="size-4" aria-hidden="true" />
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
