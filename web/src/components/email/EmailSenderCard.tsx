import { useTranslation } from 'react-i18next'
import { zodResolver } from '@hookform/resolvers/zod'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'
import { useEmailProviders } from '../../queries/emailProviders'
import {
  useUpdateEmailSettings,
  type EmailSettings,
} from '../../queries/emailSettings'
import { EmailField } from './EmailField'
import { SMTP_BACKENDS } from './emailProviderForm'
import { CARD_FLOAT, INPUT_RECESSED, PRESS } from './emailStyles'

const schema = z.object({
  fromName: z.string().trim(),
  fromAddress: z.string().trim(),
  replyTo: z.string().trim(),
  envelopeFrom: z.string().trim(),
  sendsPerMinute: z.coerce.number().int().min(0),
  sendsPerDay: z.coerce.number().int().min(0),
})
type FormInput = z.input<typeof schema>
type FormOutput = z.output<typeof schema>

function toValues(s: EmailSettings): FormInput {
  return {
    fromName: s.from_name ?? '',
    fromAddress: s.from_address ?? '',
    replyTo: s.reply_to ?? '',
    envelopeFrom: s.envelope_from ?? '',
    sendsPerMinute: s.sends_per_minute ?? 0,
    sendsPerDay: s.sends_per_day ?? 0,
  }
}

/** EmailSenderCard holds the identity every provider sends as, and the send caps. */
export function EmailSenderCard({
  settings,
}: Readonly<{ settings: EmailSettings }>) {
  const { t } = useTranslation('settings')
  const save = useUpdateEmailSettings()
  const { data: providers } = useEmailProviders()
  const smtpBased = (providers ?? []).some((p) => SMTP_BACKENDS.has(p.backend))
  const form = useForm<FormInput, unknown, FormOutput>({
    resolver: zodResolver(schema),
    values: toValues(settings),
    resetOptions: { keepDirtyValues: true },
  })
  const { register, formState, setError } = form
  const fromRequired = settings.backend !== ''

  const onSubmit = form.handleSubmit((v) => {
    if (fromRequired && !v.fromAddress) {
      setError('fromAddress', { message: 'validation.fromRequired' })
      return
    }
    save.mutate(
      {
        ...settings,
        from_name: v.fromName,
        from_address: v.fromAddress,
        reply_to: v.replyTo,
        envelope_from: v.envelopeFrom,
        sends_per_minute: v.sendsPerMinute,
        sends_per_day: v.sendsPerDay,
      },
      {
        onSuccess: () => {
          toast.add({ title: t('email.saveSuccess'), type: 'success' })
        },
      },
    )
  })

  return (
    <Card size="sm" className={CARD_FLOAT}>
      <CardContent>
        <form
          onSubmit={(e) => {
            void onSubmit(e)
          }}
          aria-label={t('emailSender.title')}
          className="space-y-3"
        >
          <div>
            <h2 className="text-sm font-semibold">{t('emailSender.title')}</h2>
            <p className="text-xs text-muted-foreground">
              {t('emailSender.description')}
            </p>
          </div>
          <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
            <EmailField
              id="from-name"
              label={t('emailDelivery.sender.fromName')}
            >
              <Input
                id="from-name"
                className={INPUT_RECESSED}
                {...register('fromName')}
              />
            </EmailField>
            <EmailField
              id="email-from-address"
              label={t('emailDelivery.sender.fromAddress')}
              error={formState.errors.fromAddress}
            >
              <Input
                id="email-from-address"
                type="email"
                className={INPUT_RECESSED}
                {...register('fromAddress')}
              />
            </EmailField>
            <EmailField
              id="reply-to"
              label={t('emailDelivery.sender.replyTo')}
              description={t('emailDelivery.sender.optional')}
            >
              <Input
                id="reply-to"
                type="email"
                className={INPUT_RECESSED}
                {...register('replyTo')}
              />
            </EmailField>
            {smtpBased ? (
              <EmailField
                id="envelope-from"
                label={t('emailDelivery.sender.envelopeFrom')}
                description={t('emailDelivery.sender.envelopeHelp')}
              >
                <Input
                  id="envelope-from"
                  type="email"
                  className={INPUT_RECESSED}
                  {...register('envelopeFrom')}
                />
              </EmailField>
            ) : null}
            <EmailField
              id="sends-per-minute"
              label={t('emailDelivery.limits.perMinute')}
              description={t('emailDelivery.limits.zeroHelp')}
            >
              <Input
                id="sends-per-minute"
                type="number"
                min={0}
                className={INPUT_RECESSED}
                {...register('sendsPerMinute')}
              />
            </EmailField>
            <EmailField
              id="sends-per-day"
              label={t('emailDelivery.limits.perDay')}
              description={t('emailDelivery.limits.zeroHelp')}
            >
              <Input
                id="sends-per-day"
                type="number"
                min={0}
                className={INPUT_RECESSED}
                {...register('sendsPerDay')}
              />
            </EmailField>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <Button
              type="submit"
              size="sm"
              className={PRESS}
              disabled={save.isPending}
            >
              {save.isPending ? t('email.saving') : t('email.save')}
            </Button>
            {save.isError ? (
              <Alert variant="destructive" className="min-w-0 flex-1">
                <AlertDescription>{save.error.message}</AlertDescription>
              </Alert>
            ) : null}
          </div>
        </form>
      </CardContent>
    </Card>
  )
}
