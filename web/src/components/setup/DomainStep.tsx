import { useState } from 'react'
import type { FormEvent } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { HelpLink } from '../HelpLink'
import {
  ingressSettingsQueryOptions,
  useUpdateIngressSettings,
} from '../../queries/domains'
import { systemDoctorQueryOptions } from '../../queries/systemDoctor'
import { useAuthUsername } from '../../hooks/useAuthUsername'
import {
  domainGate,
  normalizeDomain,
  publicIpFromDoctor,
} from '../../lib/setupWizard'
import { StepFooter } from './StepChrome'
import { DomainVerification } from './DomainVerification'
import { useDomainVerification } from './useSetupPolling'
import type { StepProps } from './types'

/** DomainStep points a domain at the dashboard and watches DNS and HTTPS come up. */
export function DomainStep({ onContinue, onSkip, pending }: StepProps) {
  const { data: settings } = useQuery(ingressSettingsQueryOptions())
  const { data: doctor } = useQuery(systemDoctorQueryOptions())
  const username = useAuthUsername()
  const update = useUpdateIngressSettings()

  const savedDomain = settings?.acme_enabled
    ? (settings.primary_domain ?? '')
    : ''
  const [domainInput, setDomainInput] = useState<string | null>(null)
  const [emailInput, setEmailInput] = useState<string | null>(null)
  const [startedAt, setStartedAt] = useState(() => Date.now())

  const domainValue = domainInput ?? savedDomain
  const emailValue =
    emailInput ??
    settings?.acme_email ??
    (username?.includes('@') ? username : '')
  const domain = normalizeDomain(domainValue)
  const saved = domain !== '' && domain === savedDomain
  const publicIp = publicIpFromDoctor(doctor)

  const { progress, budgetSpent, refetch } = useDomainVerification(
    domain,
    saved,
    startedAt,
    publicIp,
  )
  const gate = domainGate(saved, progress)

  function onSubmit(e: FormEvent) {
    e.preventDefault()
    if (!domain) return
    update.mutate(
      {
        primary_domain: domain,
        acme_enabled: true,
        acme_email: emailValue.trim(),
        acme_directory_url: settings?.acme_directory_url,
      },
      {
        onSuccess: () => {
          setDomainInput(null)
          setStartedAt(Date.now())
        },
      },
    )
  }

  const dirty =
    domain !== savedDomain ||
    (emailInput !== null && emailInput !== settings?.acme_email)
  let formHint = ''
  if (domainValue && !domain)
    formHint = 'Enter a hostname like dash.example.com, without a path.'

  return (
    <div className="space-y-4">
      <p className="max-w-prose text-sm text-muted-foreground">
        Optional, but recommended: serve this dashboard at its own domain over
        HTTPS so your password never crosses the network in plain text. You need
        a domain whose DNS you control.{' '}
        <HelpLink
          path="/domains-and-ingress"
          label="How domains work"
          variant="inline"
        />
      </p>

      <form
        onSubmit={onSubmit}
        className="grid gap-3 sm:grid-cols-[1fr_1fr_auto] sm:items-end"
      >
        <div className="space-y-1.5">
          <Label htmlFor="setup-domain">Dashboard domain</Label>
          <Input
            id="setup-domain"
            placeholder="dash.example.com"
            value={domainValue}
            onChange={(e) => setDomainInput(e.target.value)}
            autoComplete="off"
            spellCheck={false}
            aria-invalid={formHint !== ''}
          />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="setup-acme-email">
            Email for certificate notices
          </Label>
          <Input
            id="setup-acme-email"
            type="email"
            placeholder="you@example.com"
            value={emailValue}
            onChange={(e) => setEmailInput(e.target.value)}
            required
          />
        </div>
        <Button
          type="submit"
          disabled={!domain || !emailValue.trim() || update.isPending || !dirty}
        >
          {saved && !dirty ? 'Saved' : 'Save and verify'}
        </Button>
      </form>
      {formHint ? <p className="text-xs text-destructive">{formHint}</p> : null}
      {update.error ? (
        <p className="text-xs text-destructive">{update.error.message}</p>
      ) : null}

      {saved && progress ? (
        <DomainVerification
          domain={domain}
          progress={progress}
          publicIp={publicIp}
          budgetSpent={budgetSpent}
          onRecheck={() => {
            setStartedAt(Date.now())
            void refetch()
          }}
        />
      ) : null}

      <StepFooter
        gate={gate}
        onContinue={onContinue}
        onSkip={onSkip}
        pending={pending}
      />
    </div>
  )
}
