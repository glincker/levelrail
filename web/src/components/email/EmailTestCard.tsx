import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { PaperPlaneTiltIcon } from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  EmailTestError,
  useSendTestEmail,
  type EmailFailureKind,
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

/** EmailTestCard sends one real email and, on failure, shows the typed reason with a Fix. */
export function EmailTestCard({ provider }: { provider: string }) {
  const { t } = useTranslation('settings')
  const [to, setTo] = useState('')
  const send = useSendTestEmail()
  const failure =
    send.error instanceof EmailTestError &&
    KNOWN_FAILURES.includes(send.error.code)
      ? send.error.code
      : 'other'

  return (
    <Card>
      <CardContent className="space-y-3">
        <Field>
          <FieldLabel htmlFor="test-email-to">
            {t('emailDelivery.test.title')}
          </FieldLabel>
          <FieldDescription>
            {t('emailDelivery.test.description')}
          </FieldDescription>
          <div className="flex flex-col gap-2 sm:flex-row">
            <Input
              id="test-email-to"
              type="email"
              value={to}
              onChange={(e) => setTo(e.target.value)}
              placeholder={t('emailDelivery.test.placeholder')}
              className="sm:max-w-xs"
            />
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={send.isPending}
              onClick={() => send.mutate(to.trim())}
            >
              <PaperPlaneTiltIcon className="size-3.5" aria-hidden="true" />
              {send.isPending
                ? t('emailDelivery.test.sending')
                : t('emailDelivery.test.send')}
            </Button>
          </div>
        </Field>
        {send.isSuccess ? (
          <Alert>
            <AlertTitle>
              {t('emailDelivery.test.success', { to: send.data.to })}
            </AlertTitle>
            <AlertDescription>
              {t('emailDelivery.test.response', {
                provider: send.data.provider,
                id: send.data.message_id || t('emailDelivery.test.noId'),
              })}
            </AlertDescription>
          </Alert>
        ) : null}
        {send.isError ? (
          <Alert variant="destructive">
            <AlertTitle>
              {t(`emailDelivery.failure.${failure}.title`)}
            </AlertTitle>
            <AlertDescription className="space-y-1">
              <p>
                <span className="font-medium">
                  {t('emailDelivery.test.fix')}
                </span>{' '}
                {t(
                  provider === 'ses_smtp' && failure === 'auth_failed'
                    ? 'emailDelivery.failure.auth_failed.fixSesSmtp'
                    : `emailDelivery.failure.${failure}.fix`,
                )}
              </p>
              <p className="font-mono text-xs break-words opacity-80">
                {send.error.message}
              </p>
            </AlertDescription>
          </Alert>
        ) : null}
      </CardContent>
    </Card>
  )
}
