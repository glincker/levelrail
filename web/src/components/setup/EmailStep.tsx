import { useState } from 'react'
import type { FormEvent } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { toast } from '@/components/ui/toast'
import {
  emailSettingsQueryOptions,
  useSendTestEmail,
  useUpdateEmailSettings,
} from '../../queries/emailSettings'
import { emailGate } from '../../lib/setupWizard'
import { StepFooter } from './StepChrome'
import type { StepProps } from './types'

const DEFAULT_SMTP_PORT = 587

/** EmailStep saves an SMTP backend and sends a test message; SES and Resend live on the full settings page. */
export function EmailStep({ onContinue, onSkip, pending }: StepProps) {
  const { t } = useTranslation('settings')
  const { data: current } = useQuery(emailSettingsQueryOptions())
  const update = useUpdateEmailSettings()
  const sendTest = useSendTestEmail()
  const [host, setHost] = useState('')
  const [port, setPort] = useState(String(DEFAULT_SMTP_PORT))
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [from, setFrom] = useState('')
  const [testTo, setTestTo] = useState('')

  function save(e: FormEvent) {
    e.preventDefault()
    update.mutate(
      {
        backend: 'smtp',
        smtp_host: host.trim(),
        smtp_port: Number(port),
        smtp_username: username.trim(),
        smtp_password: password,
        smtp_from: from.trim(),
      },
      {
        onSuccess: () =>
          toast.add({ title: t('email.saveSuccess'), type: 'success' }),
      },
    )
  }

  function onSendTest() {
    sendTest.mutate(testTo.trim(), {
      onSuccess: () =>
        toast.add({
          title: t('email.test.success', { to: testTo.trim() }),
          type: 'success',
        }),
      onError: (error) => toast.add({ title: error.message, type: 'error' }),
    })
  }

  const configured = current?.backend ?? ''
  const canSave = host.trim() !== '' && from.trim() !== '' && Number(port) > 0

  return (
    <div className="space-y-4">
      <p className="max-w-prose text-sm text-muted-foreground">
        {t('email.wizard.intro')}
      </p>
      {configured ? (
        <p className="text-sm text-foreground" role="status">
          {t('email.wizard.configured', {
            backend: t(`email.${configured}.label`),
          })}
        </p>
      ) : null}

      <form onSubmit={save} className="grid max-w-xl gap-3 sm:grid-cols-2">
        <div className="space-y-1.5">
          <Label htmlFor="wizard-smtp-host">{t('email.smtp.host')}</Label>
          <Input
            id="wizard-smtp-host"
            value={host}
            onChange={(e) => setHost(e.target.value)}
            placeholder="smtp.example.com"
            autoComplete="off"
          />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="wizard-smtp-port">{t('email.smtp.port')}</Label>
          <Input
            id="wizard-smtp-port"
            type="number"
            value={port}
            onChange={(e) => setPort(e.target.value)}
          />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="wizard-smtp-user">{t('email.smtp.username')}</Label>
          <Input
            id="wizard-smtp-user"
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            autoComplete="off"
          />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="wizard-smtp-password">
            {t('email.smtp.password')}
          </Label>
          <Input
            id="wizard-smtp-password"
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            autoComplete="new-password"
          />
        </div>
        <div className="space-y-1.5 sm:col-span-2">
          <Label htmlFor="wizard-smtp-from">{t('email.smtp.from')}</Label>
          <Input
            id="wizard-smtp-from"
            type="email"
            value={from}
            onChange={(e) => setFrom(e.target.value)}
            placeholder="levelrail@example.com"
          />
        </div>
        {update.error ? (
          <p className="text-xs text-destructive sm:col-span-2">
            {update.error.message}
          </p>
        ) : null}
        <div className="sm:col-span-2">
          <Button type="submit" disabled={!canSave || update.isPending}>
            {update.isPending ? t('email.saving') : t('email.save')}
          </Button>
        </div>
      </form>

      {configured ? (
        <div className="max-w-xl space-y-1.5">
          <Label htmlFor="wizard-test-to">{t('email.test.title')}</Label>
          <div className="flex gap-2">
            <Input
              id="wizard-test-to"
              type="email"
              value={testTo}
              onChange={(e) => setTestTo(e.target.value)}
              placeholder={t('email.test.placeholder')}
            />
            <Button
              type="button"
              variant="outline"
              onClick={onSendTest}
              disabled={testTo.trim() === '' || sendTest.isPending}
            >
              {sendTest.isPending
                ? t('email.test.sending')
                : t('email.test.send')}
            </Button>
          </div>
        </div>
      ) : null}

      <p className="text-xs text-muted-foreground">
        {t('email.wizard.others')}{' '}
        <Link to="/settings/email" className="underline">
          {t('email.wizard.othersLink')}
        </Link>
      </p>

      <StepFooter
        gate={emailGate(configured)}
        onContinue={onContinue}
        onSkip={onSkip}
        pending={pending}
      />
    </div>
  )
}
