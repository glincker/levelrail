import { zodResolver } from '@hookform/resolvers/zod'
import { useFieldArray, useForm, useWatch } from 'react-hook-form'
import { z } from 'zod'
import { useMemo } from 'react'
import { Link } from '@tanstack/react-router'
import { GlobeIcon, PlusIcon, XIcon } from '@phosphor-icons/react/dist/ssr'
import type { AppDetail } from '../types/appDetail'
import { useUpdateApp } from '../queries/apps'
import { useCertificates } from '../queries/certificates'
import { useDebouncedValue } from '../hooks/useDebouncedValue'
import { DomainDnsCheck } from './DomainDnsCheck'
import { DomainBasicAuthControl } from './DomainBasicAuthControl'
import { certStatusMeta } from '../lib/certStatus'
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
import {
  Field,
  FieldError,
  FieldGroup,
  FieldHint,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'

const DOMAIN_PATTERN =
  /^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)+$/i

const domainSchema = z.object({
  domains: z.array(
    z.object({
      value: z
        .string()
        .trim()
        .min(1, 'Domain is required')
        .regex(DOMAIN_PATTERN, 'Enter a valid domain, e.g. app.example.com'),
    }),
  ),
})

type DomainFormValues = z.infer<typeof domainSchema>

function toFieldValues(domains?: string[]): { value: string }[] {
  return (domains ?? []).map((value) => ({ value }))
}

// Guided add flow: each domain row shows its DNS record status inline
// (reused DomainDnsCheck) and TLS status once saved, instead of both
// being a separate panel shown only after the fact.
export function DomainEditor({ app }: { app: AppDetail }) {
  const updateApp = useUpdateApp(app.name)
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
          toast.add({ title: 'Domains saved.', type: 'success' })
        },
      },
    )
  })

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <GlobeIcon className="size-4" />
          Domains
        </CardTitle>
        <CardDescription>
          Domains routed to this app through the ingress layer. Platform-wide
          ACME and primary domain settings live under{' '}
          <Link to="/domains" className="underline">
            Domains
          </Link>
          .
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form
          onSubmit={(e) => {
            void onSubmit(e)
          }}
          className="space-y-3"
        >
          <FieldHint
            href="https://developers.cloudflare.com/dns/manage-dns-records/reference/dns-record-types/"
            linkText="DNS record types explained"
          >
            Point an A record at this server&apos;s IP address (or a CNAME at
            its hostname), then a TLS certificate is issued automatically
            once it resolves.
          </FieldHint>
          {fields.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              No domains configured.
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
                          placeholder="app.example.com"
                          aria-label="Domain"
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
                        <XIcon />
                        <span className="sr-only">Remove domain</span>
                      </Button>
                    </Field>

                    {isValidDomain ? (
                      <div className="space-y-1.5 pl-1">
                        <DomainDnsCheck appName={app.name} domain={domain} />
                        {isSaved ? (
                          <>
                            <div className="flex items-center gap-2 text-xs text-muted-foreground">
                              <span>TLS certificate:</span>
                              {cert ? (
                                <Badge variant={certStatusMeta[cert.status].variant}>
                                  {certStatusMeta[cert.status].label}
                                </Badge>
                              ) : (
                                <Badge variant="muted">Provisioning</Badge>
                              )}
                            </div>
                            <DomainBasicAuthControl appName={app.name} domain={domain} />
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
              <PlusIcon />
              Add domain
            </Button>
            <Button type="submit" size="sm" disabled={updateApp.isPending}>
              {updateApp.isPending ? 'Saving...' : 'Save domains'}
            </Button>
          </div>
          {updateApp.isError ? (
            <Alert variant="destructive">
              <AlertDescription>{updateApp.error.message}</AlertDescription>
            </Alert>
          ) : null}
        </form>
      </CardContent>
    </Card>
  )
}
