import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { SendGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Field, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useEmailProviders } from '../../queries/emailProviders'
import { useSendTestEmail } from '../../queries/emailSettings'
import { DomainCheckInline } from './EmailDomainCheck'
import { EmailTestOutcome } from './EmailTestOutcome'
import { CARD_FLOAT, INPUT_RECESSED, PRESS } from './emailStyles'

/** EmailDeliveryCard sends a test through a chosen provider and checks each provider's sender domain. */
export function EmailDeliveryCard() {
  const { t } = useTranslation('settings')
  const { data } = useEmailProviders()
  const providers = data ?? []
  const [to, setTo] = useState('')
  const [picked, setPicked] = useState('')
  const send = useSendTestEmail()
  const targetId = picked || providers[0]?.id || ''
  const target = providers.find((p) => p.id === targetId)
  if (providers.length === 0) return null
  const nameOf = (backend: string, name?: string) =>
    name || t(`emailDelivery.providers.${backend as 'smtp'}`)

  return (
    <Card size="sm" id="email-delivery-check" className={CARD_FLOAT}>
      <CardContent className="space-y-3">
        <div>
          <h2 className="text-sm font-semibold">{t('emailTest.title')}</h2>
          <p className="text-xs text-muted-foreground">
            {t('emailTest.description')}
          </p>
        </div>
        <>
          <div className="flex flex-wrap items-end gap-3">
            <Field className="w-full sm:w-56">
              <FieldLabel htmlFor="test-email-provider">
                {t('emailTest.provider')}
              </FieldLabel>
              <Select
                value={targetId}
                onValueChange={(v) => {
                  setPicked(v ?? '')
                }}
              >
                <SelectTrigger
                  id="test-email-provider"
                  className={`w-full ${INPUT_RECESSED}`}
                >
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {providers.map((p) => (
                    <SelectItem key={p.id} value={p.id ?? ''}>
                      {t(`emailProviders.roles.${p.role ?? 'primary'}`)}:{' '}
                      {nameOf(p.backend, p.name)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
            <Field className="w-full sm:w-64">
              <FieldLabel htmlFor="test-email-to">
                {t('emailTest.to')}
              </FieldLabel>
              <Input
                id="test-email-to"
                type="email"
                className={INPUT_RECESSED}
                value={to}
                onChange={(e) => {
                  setTo(e.target.value)
                }}
                placeholder={t('emailDelivery.test.placeholder')}
              />
            </Field>
            <Button
              type="button"
              size="sm"
              variant="outline"
              className={PRESS}
              disabled={send.isPending}
              onClick={() => {
                send.mutate({ to: to.trim(), providerId: targetId })
              }}
            >
              <SendGlassIcon size="1.2em" aria-hidden="true" />
              {send.isPending
                ? t('emailDelivery.test.sending')
                : t('emailDelivery.test.send')}
            </Button>
          </div>
          <EmailTestOutcome
            data={send.data}
            error={send.error}
            backend={target?.backend}
          />
          <div className="grid gap-3 border-t border-border pt-3 md:grid-cols-2">
            {providers.map((p) => (
              <div key={p.id} className="min-w-0 space-y-2">
                <div className="text-sm font-medium">
                  {t('emailTest.domainFor', {
                    name: nameOf(p.backend, p.name),
                  })}
                </div>
                <DomainCheckInline providerId={p.id} backend={p.backend} />
              </div>
            ))}
          </div>
        </>
      </CardContent>
    </Card>
  )
}
