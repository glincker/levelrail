import { useTranslation } from 'react-i18next'
import { CheckGlassIcon, TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import type { GitSourceResource } from '../types/gitSource'
import { GitSourceCopyField } from './GitSourceCopyField'

// The only place the generated webhook secret is ever shown: it never
// comes back from GET, so it is rendered from the connect response alone.
export function GitSourceJustConnected({
  resource,
  autoRegistered,
  webhookError,
  onDone,
}: {
  resource: GitSourceResource
  autoRegistered: boolean
  webhookError: string | undefined
  onDone: () => void
}) {
  const { t } = useTranslation('gitSource')
  return (
    <div className="space-y-3">
      {autoRegistered ? (
        <div className="flex items-start gap-2 rounded-lg border border-emerald-200 bg-emerald-50 p-2.5 text-emerald-900 dark:border-emerald-900/50 dark:bg-emerald-900/20 dark:text-emerald-200">
          <CheckGlassIcon className="mt-0.5 size-5 shrink-0" />
          <p className="text-sm">{t('justConnected.autoRegistered')}</p>
        </div>
      ) : (
        <div className="flex items-start gap-2 rounded-lg border border-amber-200 bg-amber-50 p-2.5 text-amber-900 dark:border-amber-900/50 dark:bg-amber-900/20 dark:text-amber-200">
          <TriangleAlertGlassIcon className="mt-0.5 size-6 shrink-0 text-tone-warning" />
          <p className="text-sm">
            {webhookError ? webhookError : t('justConnected.secretOnce')}
          </p>
        </div>
      )}

      {!autoRegistered ? (
        <>
          <GitSourceCopyField
            id="git-source-webhook-url"
            label={t('justConnected.webhookUrl')}
            value={`${window.location.origin}${resource.webhook_url}`}
          />
          {resource.webhook_secret ? (
            <GitSourceCopyField
              id="git-source-webhook-secret"
              label={t('justConnected.webhookSecret')}
              value={resource.webhook_secret}
              description={t('justConnected.secretHint')}
            />
          ) : null}
        </>
      ) : null}

      <Button type="button" size="sm" onClick={onDone}>
        {t('justConnected.done')}
      </Button>
    </div>
  )
}
