import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  ArrowRightGlassIcon,
  CircleAlertGlassIcon,
  CircleCheckGlassIcon,
  LoaderCircleGlassIcon,
  RefreshCwGlassIcon,
} from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button, buttonVariants } from '@/components/ui/button'
import { Field, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'
import { isValidDomain, wwwPair } from '../../lib/domainWizard'
import {
  defaultHintProvider,
  HINT_PROVIDERS,
  isApexName,
  isWildcard,
  STEP_IDS,
  type StepId,
  type HintProvider,
} from '../../lib/domainStepper'
import { STANDALONE_SCOPE } from '../../lib/standaloneDomain'
import { useEditAppDomains } from '../../queries/appEnvironmentDomains'
import { certStatusQueryOptions } from '../../queries/domainCert'
import { useStepperDomainCheck } from '../../queries/domainStepper'
import { useServerAddress } from '../../queries/serverAddress'
import { AcmeFailureHint } from '../AcmeFailureHint'
import { DomainDnsStatusBadge } from '../DomainDnsStatusBadge'
import { DnsRecordsTable } from './ServerAddressCard'
import { StepBody, StepperNav } from './DomainStepperParts'
import { PortStep } from './DomainStepperPort'

function list(values?: string[]): string {
  return values && values.length > 0 ? values.join(', ') : ''
}

function DomainStep({
  value,
  onChange,
  valid,
  pending,
  error,
  onSubmit,
}: {
  value: string
  onChange: (v: string) => void
  valid: boolean
  pending: boolean
  error?: string
  onSubmit: () => void
}) {
  const { t } = useTranslation('domains')
  const pair = valid ? wwwPair(value) : null
  const other = pair ? (pair.typed === 'apex' ? pair.www : pair.apex) : null
  return (
    <StepBody>
      <div className="flex flex-wrap items-end gap-2">
        <Field className="min-w-48 flex-1">
          <FieldLabel htmlFor="stepper-domain">
            {t('stepper.domain.label')}
          </FieldLabel>
          <Input
            id="stepper-domain"
            className="font-mono"
            value={value}
            placeholder={t('stepper.domain.placeholder')}
            onChange={(e) => {
              onChange(e.target.value.trim().toLowerCase())
            }}
          />
        </Field>
        <Button type="button" disabled={!valid || pending} onClick={onSubmit}>
          {pending ? (
            <LoaderCircleGlassIcon
              size="1.4em"
              className="animate-spin motion-reduce:animate-none"
            />
          ) : (
            <ArrowRightGlassIcon size="1.4em" />
          )}
          {t('stepper.domain.add')}
        </Button>
      </div>
      {value !== '' && !valid ? (
        <p className="text-xs text-destructive">
          {t('stepper.domain.invalid')}
        </p>
      ) : null}
      {other ? (
        <p className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
          {t('stepper.domain.offerPair', { other })}
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() => {
              onChange(other)
            }}
          >
            {t('stepper.domain.useOther', { other })}
          </Button>
        </p>
      ) : null}
      {isWildcard(value) ? (
        <p className="text-xs text-amber-700 dark:text-amber-400">
          {t('stepper.domain.wildcard')}
        </p>
      ) : null}
      {error ? (
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      ) : null}
    </StepBody>
  )
}

function RecordsStep({
  appName,
  domain,
  onNext,
}: {
  appName: string
  domain: string
  onNext: () => void
}) {
  const { t } = useTranslation('domains')
  const address = useServerAddress(domain)
  const check = useStepperDomainCheck(appName, domain, true)
  const [picked, setPicked] = useState<HintProvider | null>(null)
  const provider = picked ?? defaultHintProvider(check.data?.dns_provider)
  return (
    <StepBody>
      <DnsRecordsTable records={address.data?.records ?? []} />
      <div
        className="flex flex-wrap items-center gap-1.5"
        role="group"
        aria-label={t('stepper.records.providerLabel')}
      >
        {HINT_PROVIDERS.map((p) => (
          <Button
            key={p}
            type="button"
            size="sm"
            variant={provider === p ? 'secondary' : 'outline'}
            aria-pressed={provider === p}
            onClick={() => {
              setPicked(p)
            }}
          >
            {t(`stepper.provider.${p}`)}
          </Button>
        ))}
      </div>
      <p className="text-xs text-muted-foreground">
        {t(`stepper.hint.${provider}`)}
        {provider === 'namecheap' && isApexName(domain)
          ? ` ${t('stepper.hint.namecheapApex')}`
          : ''}
      </p>
      <Button type="button" size="sm" onClick={onNext}>
        <ArrowRightGlassIcon size="1.4em" />
        {t('stepper.records.next')}
      </Button>
    </StepBody>
  )
}

function CheckStep({
  appName,
  domain,
  onNext,
}: {
  appName: string
  domain: string
  onNext: () => void
}) {
  const { t } = useTranslation('domains')
  const check = useStepperDomainCheck(appName, domain, true)
  const data = check.data
  const expected = list([
    ...(data?.expected_host ? [data.expected_host] : []),
    ...(data?.expected_ipv4 ?? []),
    ...(data?.expected_ipv6 ?? []),
  ])
  const connected = data?.status === 'connected'
  return (
    <StepBody>
      <div className="flex flex-wrap items-center gap-2 text-xs">
        <DomainDnsStatusBadge status={data?.status} />
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={check.isFetching}
          onClick={check.recheck}
        >
          <RefreshCwGlassIcon
            size="1.4em"
            className={
              check.isFetching ? 'animate-spin motion-reduce:animate-none' : ''
            }
          />
          {t('stepper.check.recheck')}
        </Button>
        {connected ? (
          <Button type="button" size="sm" onClick={onNext}>
            <ArrowRightGlassIcon size="1.4em" />
            {t('stepper.check.next')}
          </Button>
        ) : null}
      </div>
      <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-xs">
        <dt className="text-muted-foreground">{t('stepper.check.expected')}</dt>
        <dd className="font-mono break-all">
          {expected || t('stepper.check.none')}
        </dd>
        <dt className="text-muted-foreground">{t('stepper.check.resolved')}</dt>
        <dd className="font-mono break-all">
          {list(data?.resolved_hosts) || t('stepper.check.none')}
        </dd>
      </dl>
      {connected ? null : (
        <p className="text-xs text-muted-foreground">
          {check.capped
            ? t('stepper.check.capped')
            : t('stepper.check.propagation')}
        </p>
      )}
      {check.isError ? (
        <p className="text-xs text-destructive">{check.error.message}</p>
      ) : null}
    </StepBody>
  )
}

function CertificateStep({
  appName,
  domain,
}: {
  appName: string
  domain: string
}) {
  const { t } = useTranslation('domains')
  const cert = useQuery(certStatusQueryOptions(domain, true))
  const check = useStepperDomainCheck(appName, domain, true)
  const status = cert.data
  const state = status?.state ?? 'pending'
  return (
    <StepBody>
      <p
        role="status"
        className="flex items-center gap-1.5 text-sm font-medium"
      >
        {state === 'issued' ? (
          <CircleCheckGlassIcon
            className="size-5 text-emerald-600 dark:text-emerald-400"
            aria-hidden="true"
          />
        ) : state === 'failed' ? (
          <CircleAlertGlassIcon
            className="size-5 text-destructive"
            aria-hidden="true"
          />
        ) : (
          <LoaderCircleGlassIcon
            className="size-5 animate-spin motion-reduce:animate-none"
            aria-hidden="true"
          />
        )}
        {t(`stepper.cert.${state}`)}
      </p>
      {status?.state === 'failed' ? (
        <div className="space-y-1 text-xs">
          {status.error ? (
            <p className="break-words">
              <span className="text-muted-foreground">
                {t('stepper.cert.reason')}:{' '}
              </span>
              <code className="font-mono">{status.error}</code>
            </p>
          ) : null}
          {status.explanation ? <p>{status.explanation}</p> : null}
          {status.hint ? (
            <p className="text-muted-foreground">{status.hint}</p>
          ) : null}
        </div>
      ) : null}
      {check.data?.acme_failure ? (
        <AcmeFailureHint failure={check.data.acme_failure} />
      ) : null}
      {appName !== STANDALONE_SCOPE && state !== 'issued' ? (
        <Link
          to="/apps/$name/logs"
          params={{ name: appName }}
          className={buttonVariants({ size: 'sm', variant: 'outline' })}
        >
          {t('stepper.cert.logs')}
        </Link>
      ) : null}
    </StepBody>
  )
}

// A 4-step guide: enter the domain, add the DNS records, watch DNS resolve,
// then watch the certificate. Pass initialDomain when it is already added.
export function DomainStepper({
  appName,
  initialDomain = '',
}: {
  appName: string
  initialDomain?: string
}) {
  const { t } = useTranslation('domains')
  const edit = useEditAppDomains(appName)
  const [domain, setDomain] = useState(initialDomain)
  const [step, setStep] = useState<StepId>(initialDomain ? 'records' : 'domain')
  const [reached, setReached] = useState(initialDomain ? 1 : 0)
  const valid = isValidDomain(domain)

  function go(id: StepId) {
    setStep(id)
    setReached((r) => Math.max(r, STEP_IDS.indexOf(id)))
  }

  function add() {
    edit.mutate(
      { add: [domain] },
      {
        onSuccess: () => {
          toast.add({ title: t('save.done', { domain }), type: 'success' })
          go('records')
        },
      },
    )
  }

  return (
    <div className="space-y-3 rounded-lg border border-border bg-muted/20 p-3">
      <StepperNav current={step} reached={reached} onGo={go} />
      {step === 'domain' ? (
        <DomainStep
          value={domain}
          onChange={setDomain}
          valid={valid}
          pending={edit.isPending}
          error={edit.isError ? edit.error.message : undefined}
          onSubmit={add}
        />
      ) : null}
      {step === 'records' ? (
        <RecordsStep
          appName={appName}
          domain={domain}
          onNext={() => {
            go('check')
          }}
        />
      ) : null}
      {step === 'check' ? (
        <CheckStep
          appName={appName}
          domain={domain}
          onNext={() => {
            go(appName === STANDALONE_SCOPE ? 'certificate' : 'port')
          }}
        />
      ) : null}
      {step === 'port' ? (
        <PortStep
          appName={appName}
          onNext={() => {
            go('certificate')
          }}
        />
      ) : null}
      {step === 'certificate' ? (
        <CertificateStep appName={appName} domain={domain} />
      ) : null}
    </div>
  )
}
