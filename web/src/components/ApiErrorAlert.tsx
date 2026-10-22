import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQueryClient } from '@tanstack/react-query'
import { CopyIcon, WarningIcon } from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { ApiError } from '../lib/apiError'
import { useApplyFix } from '../queries/fixit'
import { dockerIntegrationKeys } from '../queries/dockerIntegration'

const DOCKER_CODE = 'docker_unavailable'

// Renders any request error: typed ones get their fix or re-check button,
// generic ones get a request id the operator can quote.
export function ApiErrorAlert({
  error,
  onFixed,
}: {
  error: unknown
  onFixed?: () => void
}) {
  const { t } = useTranslation('common')
  const qc = useQueryClient()
  const apply = useApplyFix()
  const [copied, setCopied] = useState(false)
  const api = error instanceof ApiError ? error : undefined
  const message =
    error instanceof Error && error.message !== 'internal error'
      ? error.message
      : t('apiError.generic')

  async function runFix(id: string) {
    await apply.mutateAsync({ id })
    onFixed?.()
  }

  function recheck() {
    void qc
      .invalidateQueries({ queryKey: dockerIntegrationKeys.health })
      .then(() => onFixed?.())
  }

  function copyId(id: string) {
    void navigator.clipboard
      .writeText(id)
      .then(() => {
        setCopied(true)
      })
      .catch(() => undefined)
  }

  return (
    <Alert variant="destructive" data-testid="api-error-alert">
      <WarningIcon aria-hidden="true" />
      <AlertDescription className="space-y-2">
        <p>{message}</p>
        <div className="flex flex-wrap items-center gap-2">
          {api?.fix ? (
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={apply.isPending}
              onClick={() => {
                void runFix(api.fix?.id ?? '')
              }}
            >
              {apply.isPending ? t('apiError.fixing') : api.fix.label}
            </Button>
          ) : null}
          {api?.code === DOCKER_CODE ? (
            <Button type="button" size="sm" variant="outline" onClick={recheck}>
              {t('apiError.recheckDocker')}
            </Button>
          ) : null}
          {api?.requestId ? (
            <Button
              type="button"
              size="sm"
              variant="ghost"
              aria-label={t('apiError.copyRequestId')}
              onClick={() => {
                copyId(api.requestId ?? '')
              }}
            >
              <CopyIcon aria-hidden="true" />
              {copied
                ? t('apiError.copied')
                : t('apiError.requestId', { id: api.requestId })}
            </Button>
          ) : null}
        </div>
        {apply.isError ? <p>{t('apiError.fixFailed')}</p> : null}
      </AlertDescription>
    </Alert>
  )
}
