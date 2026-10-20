import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import { CheckIcon, CopyIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { cn } from '@/lib/utils'
import { useCopyToClipboard } from '../../hooks/useCopyToClipboard'
import { useCreateDnsZone } from '../../queries/dns'
import type { DnsZone } from '../../types/dns'
import { DelegationPanel } from './DelegationPanel'
import { ImportExisting } from './ImportExisting'

const WIZARD_STEPS = ['domain', 'nameservers', 'import', 'verify'] as const
type WizardStep = (typeof WIZARD_STEPS)[number]

const DOMAIN = /^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$/

function Stepper({ step }: { step: WizardStep }) {
  const { t } = useTranslation('dns')
  const current = WIZARD_STEPS.indexOf(step)
  return (
    <ol
      className="flex flex-wrap gap-2 text-xs"
      aria-label={t('wizard.progress')}
    >
      {WIZARD_STEPS.map((s, i) => (
        <li
          key={s}
          aria-current={s === step ? 'step' : undefined}
          className={cn(
            'flex items-center gap-1.5 rounded-md border px-2 py-1',
            s === step
              ? 'border-primary text-foreground'
              : 'border-border text-muted-foreground',
          )}
        >
          <span className="font-mono">
            {i < current ? <CheckIcon /> : i + 1}
          </span>
          {t(`wizard.step.${s}`)}
        </li>
      ))}
    </ol>
  )
}

function NameServerList({ servers }: { servers: string[] }) {
  const { t } = useTranslation('dns')
  const { copied, copy } = useCopyToClipboard()
  return (
    <div className="space-y-2">
      <ul className="space-y-1 rounded-md border border-border p-3 font-mono text-sm">
        {servers.map((ns) => (
          <li key={ns}>{ns}</li>
        ))}
      </ul>
      <Button
        size="sm"
        variant="outline"
        onClick={() => copy(servers.join('\n'))}
      >
        {copied ? <CheckIcon /> : <CopyIcon />}
        {copied ? t('wizard.copied') : t('wizard.copyAll')}
      </Button>
    </div>
  )
}

/** DelegationWizard walks an operator from a domain to a delegated zone: create, set name servers, import, verify. */
export function DelegationWizard({
  open,
  onOpenChange,
  existing,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  existing?: DnsZone
}) {
  const { t } = useTranslation('dns')
  const [step, setStep] = useState<WizardStep>(
    existing ? 'nameservers' : 'domain',
  )
  const [domain, setDomain] = useState('')
  const [zone, setZone] = useState<DnsZone | undefined>(existing)
  const create = useCreateDnsZone()
  const valid = DOMAIN.test(domain.trim().toLowerCase())

  function submitDomain() {
    create.mutate(
      { name: domain.trim().toLowerCase() },
      {
        onSuccess: (z) => {
          setZone(z)
          setStep('nameservers')
        },
      },
    )
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>
            {zone
              ? t('wizard.titleFor', { zone: zone.name })
              : t('wizard.title')}
          </DialogTitle>
          <DialogDescription>{t(`wizard.lead.${step}`)}</DialogDescription>
        </DialogHeader>
        <Stepper step={step} />

        {step === 'domain' ? (
          <div className="space-y-3">
            <div className="space-y-1.5">
              <Label htmlFor="dns-wizard-domain">{t('wizard.domain')}</Label>
              <Input
                id="dns-wizard-domain"
                value={domain}
                placeholder="example.com"
                autoComplete="off"
                spellCheck={false}
                className="font-mono"
                aria-invalid={domain !== '' && !valid}
                onChange={(e) => setDomain(e.target.value)}
              />
              <p className="text-xs text-muted-foreground">
                {t('wizard.domainHint')}
              </p>
            </div>
            {create.error ? (
              <p className="text-sm text-destructive" role="alert">
                {create.error.message}
              </p>
            ) : null}
            <div className="flex justify-end">
              <Button
                disabled={!valid || create.isPending}
                onClick={submitDomain}
              >
                {create.isPending ? t('wizard.creating') : t('wizard.create')}
              </Button>
            </div>
          </div>
        ) : null}

        {step === 'nameservers' && zone ? (
          <div className="space-y-3">
            <NameServerList servers={zone.name_servers} />
            <p className="text-sm text-muted-foreground">
              {t('wizard.registrarHint')}
            </p>
            <div className="flex justify-end gap-2">
              <Button variant="outline" onClick={() => setStep('verify')}>
                {t('wizard.skipImport')}
              </Button>
              <Button onClick={() => setStep('import')}>
                {t('wizard.next')}
              </Button>
            </div>
          </div>
        ) : null}

        {step === 'import' && zone ? (
          <ImportExisting zone={zone.id} onDone={() => setStep('verify')} />
        ) : null}

        {step === 'verify' && zone ? (
          <div className="space-y-3">
            <DelegationPanel zone={zone.id} />
            <p className="text-xs text-muted-foreground">
              {t('wizard.propagationHint')}
            </p>
            <div className="flex justify-end">
              <Button
                render={<Link to="/dns/$zone" params={{ zone: zone.name }} />}
                nativeButton={false}
                onClick={() => onOpenChange(false)}
              >
                {t('wizard.openZone')}
              </Button>
            </div>
          </div>
        ) : null}
      </DialogContent>
    </Dialog>
  )
}
