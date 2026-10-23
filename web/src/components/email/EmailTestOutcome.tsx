import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import {
  EmailTestError,
  type EmailFailureKind,
  type EmailTestResult,
} from '../../queries/emailSettings'

const KNOWN_FAILURES: EmailFailureKind[] = [
  'auth_failed',
  'sender_not_verified',
  'region_mismatch',
  'port_blocked',
  'tls_error',
  'rate_limited',
  'not_configured',
  'other',
]

interface Props {
  data?: EmailTestResult
  error?: Error | null
  /** Backend of the provider that was tested, to pick the SES SMTP credential hint. */
  backend?: string
}

/** EmailTestOutcome announces a test send result (live region) with the typed reason and its fix. */
export function EmailTestOutcome({ data, error, backend }: Readonly<Props>) {
  const { t } = useTranslation('settings')
  const failure =
    error instanceof EmailTestError && KNOWN_FAILURES.includes(error.code)
      ? error.code
      : 'other'

  return (
    <div aria-live="polite" role="status" className="empty:hidden">
      {data ? (
        <Alert>
          <AlertTitle>
            {t('emailDelivery.test.success', { to: data.to })}
          </AlertTitle>
          <AlertDescription>
            {t('emailDelivery.test.response', {
              provider: data.provider,
              id: data.message_id || t('emailDelivery.test.noId'),
            })}
          </AlertDescription>
        </Alert>
      ) : null}
      {error ? (
        <Alert variant="destructive">
          <AlertTitle>{t(`emailDelivery.failure.${failure}.title`)}</AlertTitle>
          <AlertDescription className="space-y-1">
            <p>
              <span className="font-medium">{t('emailDelivery.test.fix')}</span>{' '}
              {t(
                backend === 'ses_smtp' && failure === 'auth_failed'
                  ? 'emailDelivery.failure.auth_failed.fixSesSmtp'
                  : `emailDelivery.failure.${failure}.fix`,
              )}
            </p>
            <p className="font-mono text-xs break-words opacity-80">
              {error.message}
            </p>
          </AlertDescription>
        </Alert>
      ) : null}
    </div>
  )
}
