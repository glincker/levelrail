import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Field, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'
import type { AppDetail } from '../types/appDetail'
import { useUpdateApp } from '../queries/apps'
import { useDomainCheck } from '../queries/domainCheck'
import { setDomainRedirect } from '../queries/domainRedirect'
import {
  useAppEnvironmentDomains,
  useEditAppDomains,
} from '../queries/appEnvironmentDomains'
import {
  useIngressConnectivity,
  useListeningPorts,
} from '../queries/domainWizard'
import { isValidDomain, redirectPair, wwwPair } from '../lib/domainWizard'
import { useDebouncedValue } from '../hooks/useDebouncedValue'
import { DomainDnsCheck } from './DomainDnsCheck'
import {
  CertificateStepBody,
  PortStepBody,
  WizardStep,
} from './DomainWizardSteps'

// One guided flow: domain, live DNS check, container port, certificate
// path. Every check runs against the typed domain before it is saved.
export function DomainWizard({ app }: { app: AppDetail }) {
  const { t } = useTranslation('domains')
  const [domain, setDomain] = useState('')
  const [environment, setEnvironment] = useState('')
  const [preset, setPreset] = useState(false)
  const typed = domain.trim().toLowerCase()
  const debounced = useDebouncedValue(typed, 400)
  const valid = isValidDomain(debounced) && debounced === typed

  const envDomains = useAppEnvironmentDomains(app.name)
  const check = useDomainCheck(app.name, valid ? typed : '')
  const connectivity = useIngressConnectivity(valid)
  const listening = useListeningPorts(app.name, valid)
  const updateApp = useUpdateApp(app.name)
  const edit = useEditAppDomains(app.name)

  const pair = valid ? wwwPair(typed) : null
  const presetPair = pair ? redirectPair(pair) : null

  async function applyRedirect(pairToApply: { from: string; to: string }) {
    try {
      await setDomainRedirect(app.name, pairToApply.from, {
        target_url: `https://${pairToApply.to}`,
        status_code: 301,
      })
      toast.add({
        title: t('save.redirectDone', pairToApply),
        type: 'success',
      })
    } catch {
      toast.add({ title: t('save.failed'), type: 'error' })
    }
  }

  function submit() {
    const add = [typed]
    if (preset && presetPair) add.push(presetPair.from)
    edit.mutate(
      { add, environment: environment || undefined },
      {
        onSuccess: () => {
          toast.add({
            title: t('save.done', { domain: typed }),
            type: 'success',
          })
          if (preset && presetPair) {
            void applyRedirect(presetPair)
          }
          setDomain('')
          setPreset(false)
        },
      },
    )
  }

  return (
    <div className="space-y-3 rounded-lg border border-border bg-muted/20 p-3">
      <div>
        <h3 className="text-sm font-semibold text-foreground">
          {t('wizard.title')}
        </h3>
        <p className="text-xs text-muted-foreground">
          {t('wizard.description')}
        </p>
      </div>

      <WizardStep index={1} title={t('wizard.step.domain')}>
        <Field>
          <FieldLabel htmlFor="wizard-domain">
            {t('wizard.domain.label')}
          </FieldLabel>
          <Input
            id="wizard-domain"
            className="font-mono"
            value={domain}
            placeholder={t('wizard.domain.placeholder')}
            onChange={(e) => {
              setDomain(e.target.value)
            }}
          />
        </Field>
        {typed !== '' && !isValidDomain(typed) ? (
          <p className="text-xs text-destructive">
            {t('wizard.domain.invalid')}
          </p>
        ) : null}
        {envDomains.data && envDomains.data.environments.length > 0 ? (
          <Field>
            <FieldLabel htmlFor="wizard-env">
              {t('wizard.domain.environmentLabel')}
            </FieldLabel>
            <select
              id="wizard-env"
              value={environment}
              onChange={(e) => {
                setEnvironment(e.target.value)
              }}
              className="h-8 w-full rounded-md border border-input bg-background px-2 text-sm"
            >
              <option value="">{t('wizard.domain.environmentDefault')}</option>
              {envDomains.data.environments.map((env) => (
                <option key={env.environment_id} value={env.environment_id}>
                  {env.name}
                </option>
              ))}
            </select>
          </Field>
        ) : null}
        {pair && presetPair ? (
          <label className="flex items-start gap-2 text-xs">
            <Checkbox
              checked={preset}
              onCheckedChange={(c) => {
                setPreset(c === true)
              }}
            />
            <span>
              {pair.typed === 'apex'
                ? t('wizard.domain.wwwPreset', {
                    www: pair.www,
                    apex: pair.apex,
                  })
                : t('wizard.domain.apexPreset', {
                    apex: pair.apex,
                    www: pair.www,
                  })}
            </span>
          </label>
        ) : null}
      </WizardStep>

      <WizardStep index={2} title={t('wizard.step.dns')}>
        {valid ? (
          <>
            <DomainDnsCheck appName={app.name} domain={typed} />
            <p className="text-xs text-muted-foreground">
              {t('wizard.dns.propagation')}
            </p>
          </>
        ) : (
          <p className="text-xs text-muted-foreground">
            {t('wizard.dns.needDomain')}
          </p>
        )}
      </WizardStep>

      <WizardStep index={3} title={t('wizard.step.port')}>
        <PortStepBody
          appPort={app.port}
          data={listening.data}
          isFetching={listening.isFetching}
          switching={updateApp.isPending}
          onSwitch={(port) => {
            updateApp.mutate(
              { ...app, port },
              {
                onSuccess: () => {
                  toast.add({ title: t('port.saved'), type: 'success' })
                  void listening.refetch()
                },
              },
            )
          }}
          onRecheck={() => {
            void listening.refetch()
          }}
        />
      </WizardStep>

      <WizardStep index={4} title={t('wizard.step.certificate')}>
        <CertificateStepBody
          domain={typed}
          check={check.data}
          connectivity={connectivity.data}
        />
      </WizardStep>

      {edit.isError ? (
        <Alert variant="destructive">
          <AlertDescription>{edit.error.message}</AlertDescription>
        </Alert>
      ) : null}
      <Button
        type="button"
        size="sm"
        disabled={!valid || edit.isPending}
        onClick={submit}
      >
        {edit.isPending ? t('save.submitting') : t('save.submit')}
      </Button>
    </div>
  )
}
