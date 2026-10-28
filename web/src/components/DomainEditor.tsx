import { zodResolver } from '@hookform/resolvers/zod'
import { useFieldArray, useForm, useWatch } from 'react-hook-form'
import { z } from 'zod'
import { useMemo, useState } from 'react'
import { Link } from '@tanstack/react-router'
import { Trans, useTranslation } from 'react-i18next'
import { GlobeIcon, PlusIcon, XIcon } from '@phosphor-icons/react/dist/ssr'
import { DOMAIN_PATTERN } from '../lib/domainWizard'
import { DomainEnvironmentTabs } from './DomainEnvironmentTabs'
import { DomainStepper } from './domains/DomainStepper'
import type { AppDetail } from '../types/appDetail'
import { useUpdateApp } from '../queries/apps'
import { useCertificates } from '../queries/certificates'
import { useDebouncedValue } from '../hooks/useDebouncedValue'
import { AutomaticDomainRow } from './AutomaticDomainRow'
import { DomainDnsCheck } from './DomainDnsCheck'
import { ProxyDomainStatus, ProxyHandledTlsBadge } from './ProxyDomainStatus'
import { DomainDnsRecordsControl } from './DomainDnsRecordsControl'
import { DomainBasicAuthControl } from './DomainBasicAuthControl'
import { DomainAuthGateControl } from './DomainAuthGateControl'
import { DomainErrorPagesControl } from './DomainErrorPagesControl'
import { DomainMaintenanceControl } from './DomainMaintenanceControl'
import { DomainRedirectControl } from './DomainRedirectControl'
import { DomainSearchVisibilityControl } from './DomainSearchVisibilityControl'
import { DomainTLSCertControl } from './DomainTLSCertControl'
import { DomainWafControl } from './DomainWafControl'
import { certRenewalBadge } from '../lib/certStatus'
import { certTruth } from '../lib/certTruth'
import { CertTruthBadge } from './CertTruthBadge'
import { GetCertificateButton } from './GetCertificateButton'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Field, FieldError, FieldGroup, FieldHint } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'

// Built per render so validation messages follow the active language.
function buildDomainSchema(
  t: (key: 'editor.errors.required' | 'editor.errors.invalid') => string,
) {
  return z.object({
    domains: z.array(
      z.object({
        value: z
          .string()
          .trim()
          .min(1, t('editor.errors.required'))
          .regex(DOMAIN_PATTERN, t('editor.errors.invalid')),
      }),
    ),
  })
}

type DomainFormValues = z.infer<ReturnType<typeof buildDomainSchema>>

function toFieldValues(domains?: string[]): { value: string }[] {
  return (domains ?? []).map((value) => ({ value }))
}

// Guided add flow: each domain row shows its DNS record status inline
// (reused DomainDnsCheck) and TLS status once saved, instead of both
// being a separate panel shown only after the fact.
export function DomainEditor({ app }: { app: AppDetail }) {
  const { t } = useTranslation('domains')
  const [wizardOpen, setWizardOpen] = useState(false)
  const updateApp = useUpdateApp(app.name)
  const domainSchema = useMemo(() => buildDomainSchema(t), [t])
  const { data: certificates } = useCertificates()
  const { control, register, handleSubmit, formState } =
    useForm<DomainFormValues>({
      resolver: zodResolver(domainSchema),
      values: { domains: toFieldValues(app.domains) },
      // keepDirtyValues re-syncs untouched fields on save without
      // discarding edits still in progress here or in EnvEditor.
      resetOptions: { keepDirtyValues: true },
    })
  const { fields, append, remove } = useFieldArray({
    control,
    name: 'domains',
  })
  const watchedDomains = useWatch({ control, name: 'domains' }) ?? []
  const debouncedDomains = useDebouncedValue(watchedDomains, 400)
  const savedDomains = useMemo(() => new Set(app.domains ?? []), [app.domains])
  const certByDomain = useMemo(() => {
    const m = new Map<string, (typeof certificates)[number]>()
    for (const cert of certificates) {
      m.set(cert.domain, cert)
    }
    return m
  }, [certificates])

  const onSubmit = handleSubmit((values) => {
    updateApp.mutate(
      {
        ...app,
        domains: values.domains.map((d) => d.value.trim()),
      },
      {
        onSuccess: () => {
          toast.add({ title: t('editor.saved'), type: 'success' })
        },
      },
    )
  })

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <GlobeIcon className="size-4" aria-hidden="true" />
          {t('editor.title')}
        </CardTitle>
        <CardDescription>
          <Trans
            t={t}
            i18nKey="editor.description"
            components={{ link: <Link to="/domains" className="underline" /> }}
          />
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="flex justify-end">
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => {
              setWizardOpen((open) => !open)
            }}
          >
            {wizardOpen ? t('wizard.close') : t('wizard.open')}
          </Button>
        </div>
        {wizardOpen ? <DomainStepper appName={app.name} /> : null}
        <DomainEnvironmentTabs appName={app.name}>
          <form
            onSubmit={(e) => {
              void onSubmit(e)
            }}
            className="space-y-3"
          >
            <FieldHint
              href="https://developers.cloudflare.com/dns/manage-dns-records/reference/dns-record-types/"
              linkText={t('editor.dnsHintLink')}
            >
              {t('editor.dnsHint')}
            </FieldHint>
            <AutomaticDomainRow appName={app.name} />
            {fields.length === 0 ? (
              <p className="text-sm text-muted-foreground">
                {t('editor.empty')}
              </p>
            ) : (
              <FieldGroup className="gap-3">
                {fields.map((field, index) => {
                  const domain = debouncedDomains[index]?.value?.trim() ?? ''
                  const isValidDomain = DOMAIN_PATTERN.test(domain)
                  const isSaved = savedDomains.has(domain)
                  const cert = isSaved ? certByDomain.get(domain) : undefined

                  return (
                    <div key={field.id} className="space-y-2">
                      <Field orientation="horizontal">
                        <div className="flex-1">
                          <Input
                            {...register(`domains.${index}.value`)}
                            className="font-mono"
                            placeholder={t('editor.inputPlaceholder')}
                            aria-label={t('editor.inputLabel')}
                          />
                          <FieldError
                            errors={
                              formState.errors.domains?.[index]?.value
                                ? [formState.errors.domains[index]?.value]
                                : undefined
                            }
                          />
                        </div>
                        <Button
                          type="button"
                          variant="ghost"
                          size="icon-sm"
                          onClick={() => {
                            remove(index)
                          }}
                        >
                          <XIcon aria-hidden="true" />
                          <span className="sr-only">{t('editor.remove')}</span>
                        </Button>
                      </Field>

                      {isValidDomain ? (
                        <div className="space-y-1.5 pl-1">
                          <DomainDnsCheck appName={app.name} domain={domain} />
                          {isSaved ? (
                            <>
                              <div className="flex items-center gap-2 text-xs text-muted-foreground">
                                <span>{t('editor.tlsLabel')}</span>
                                {cert ? (
                                  <>
                                    <CertTruthBadge cert={cert} />
                                    {certRenewalBadge(cert) ? (
                                      <Badge
                                        variant="destructive"
                                        title={certRenewalBadge(cert)?.hint}
                                      >
                                        {certRenewalBadge(cert)?.label}
                                      </Badge>
                                    ) : null}
                                    {certTruth(cert).wantsRealCert ? (
                                      <GetCertificateButton domain={domain} />
                                    ) : null}
                                  </>
                                ) : (
                                  <ProxyHandledTlsBadge
                                    domain={domain}
                                    fallback={
                                      <Badge variant="muted">
                                        {t('editor.provisioning')}
                                      </Badge>
                                    }
                                  />
                                )}
                              </div>
                              <ProxyDomainStatus domain={domain} />
                              <DomainDnsRecordsControl
                                appName={app.name}
                                domain={domain}
                              />
                              <DomainBasicAuthControl
                                appName={app.name}
                                domain={domain}
                              />
                              <DomainAuthGateControl
                                appName={app.name}
                                domain={domain}
                              />
                              <DomainMaintenanceControl
                                appName={app.name}
                                domain={domain}
                              />
                              <DomainRedirectControl
                                appName={app.name}
                                domain={domain}
                              />
                              <DomainTLSCertControl
                                appName={app.name}
                                domain={domain}
                              />
                              <DomainWafControl
                                appName={app.name}
                                domain={domain}
                              />
                              <DomainErrorPagesControl
                                appName={app.name}
                                domain={domain}
                              />
                              <DomainSearchVisibilityControl
                                appName={app.name}
                                domain={domain}
                              />
                            </>
                          ) : null}
                        </div>
                      ) : null}
                    </div>
                  )
                })}
              </FieldGroup>
            )}
            <div className="flex items-center gap-2 pt-1">
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => {
                  append({ value: '' })
                }}
              >
                <PlusIcon aria-hidden="true" />
                {t('editor.add')}
              </Button>
              <Button type="submit" size="sm" disabled={updateApp.isPending}>
                {updateApp.isPending ? t('editor.saving') : t('editor.save')}
              </Button>
            </div>
            {updateApp.isError ? (
              <Alert variant="destructive">
                <AlertDescription>{updateApp.error.message}</AlertDescription>
              </Alert>
            ) : null}
          </form>
        </DomainEnvironmentTabs>
      </CardContent>
    </Card>
  )
}
