import { zodResolver } from '@hookform/resolvers/zod'
import { Controller, useForm } from 'react-hook-form'
import { z } from 'zod'
import { GlobeIcon } from '@phosphor-icons/react/dist/ssr'
import type { CertificateStatus } from '../queries/certificates'
import type { IngressSettings } from '../queries/domains'
import { useUpdateIngressSettings } from '../queries/domains'
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
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'

const domainPattern =
  /^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)+$/i

// Same shape backend validation enforces (internal/api/ingress_settings.go's
// validateIngressSettingsRequest): acmeEmail is only required, and only
// checked, when acmeEnabled is true. primaryDomain and
// acmeDirectoryUrl are both genuinely optional either way, matching
// store.IngressSettings' own "empty is a real, valid state" fields.
const ingressSettingsSchema = z
  .object({
    primaryDomain: z.string().trim(),
    acmeEnabled: z.boolean(),
    acmeEmail: z.string().trim(),
    acmeDirectoryUrl: z.string().trim(),
  })
  .superRefine((data, ctx) => {
    if (data.primaryDomain && !domainPattern.test(data.primaryDomain)) {
      ctx.addIssue({
        code: 'custom',
        message: 'Enter a valid domain, e.g. dashboard.example.com',
        path: ['primaryDomain'],
      })
    }
    if (data.acmeEnabled) {
      if (!data.acmeEmail) {
        ctx.addIssue({
          code: 'custom',
          message: 'An email is required to enable ACME',
          path: ['acmeEmail'],
        })
      } else if (!z.string().email().safeParse(data.acmeEmail).success) {
        ctx.addIssue({
          code: 'custom',
          message: 'Enter a valid email address',
          path: ['acmeEmail'],
        })
      }
    }
  })

type IngressSettingsFormValues = z.infer<typeof ingressSettingsSchema>

function toFieldValues(settings: IngressSettings): IngressSettingsFormValues {
  return {
    primaryDomain: settings.primary_domain ?? '',
    acmeEnabled: settings.acme_enabled,
    acmeEmail: settings.acme_email ?? '',
    acmeDirectoryUrl: settings.acme_directory_url ?? '',
  }
}

function toIngressSettings(
  values: IngressSettingsFormValues,
): IngressSettings {
  return {
    primary_domain: values.primaryDomain,
    acme_enabled: values.acmeEnabled,
    acme_email: values.acmeEmail,
    acme_directory_url: values.acmeDirectoryUrl,
  }
}

const certStatusMeta: Record<
  CertificateStatus['status'],
  { label: string; variant: 'success' | 'warning' | 'destructive' }
> = {
  healthy: { label: 'Healthy', variant: 'success' },
  expiring_soon: { label: 'Expiring soon', variant: 'warning' },
  expired: { label: 'Expired', variant: 'destructive' },
}

// IngressSettingsCard is the platform-wide half of the Domains page: the
// primary domain the control plane's own dashboard is reachable on, and
// the global ACME toggle (store.IngressSettings, PUT
// /api/v1/settings/ingress). ADR 005's own "Verified" section names the
// gap this closes: real ACME issuance was explicitly unproven until
// spot-checked against a real domain, not the default behavior. Off is
// still the default here, matching the backend's own zero-value default
// exactly.
//
// primaryCert is the certificate (if any) matching the current
// primary_domain, looked up by the caller (routes/domains/index.tsx)
// against the already-fetched certificates list by domain string, the
// same client-side merge the app-domains table below does per row: no
// second network request for a single cert lookup.
export function IngressSettingsCard({
  settings,
  primaryCert,
}: {
  settings: IngressSettings
  primaryCert?: CertificateStatus
}) {
  const updateSettings = useUpdateIngressSettings()
  const { control, register, handleSubmit, formState } =
    useForm<IngressSettingsFormValues>({
      resolver: zodResolver(ingressSettingsSchema),
      values: toFieldValues(settings),
      resetOptions: { keepDirtyValues: true },
    })

  const onSubmit = handleSubmit((values) => {
    updateSettings.mutate(toIngressSettings(values), {
      onSuccess: () => {
        toast.add({ title: 'Ingress settings saved.', type: 'success' })
      },
    })
  })

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <GlobeIcon className="size-4" />
          Platform ingress
        </CardTitle>
        <CardDescription>
          The dashboard&apos;s own domain, and whether certificates are
          issued by a real ACME certificate authority instead of this
          platform&apos;s offline, self-signed one.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form
          onSubmit={(e) => {
            void onSubmit(e)
          }}
          className="space-y-5"
        >
          <FieldGroup className="gap-2">
            <Field>
              <FieldLabel htmlFor="primary-domain">
                Primary domain
              </FieldLabel>
              <Input
                id="primary-domain"
                {...register('primaryDomain')}
                className="font-mono"
                placeholder="dashboard.example.com"
              />
              <FieldDescription>
                Routes the dashboard itself through a real domain, in
                addition to its existing address.
              </FieldDescription>
              <FieldError errors={[formState.errors.primaryDomain]} />
            </Field>
            {settings.primary_domain && primaryCert ? (
              <div className="flex items-center gap-2 text-sm">
                <span className="text-muted-foreground">
                  Certificate status:
                </span>
                <Badge variant={certStatusMeta[primaryCert.status].variant}>
                  {certStatusMeta[primaryCert.status].label}
                </Badge>
              </div>
            ) : null}
          </FieldGroup>

          <div className="rounded-md border border-border p-3">
            <Field orientation="horizontal">
              <Controller
                control={control}
                name="acmeEnabled"
                render={({ field }) => (
                  <Switch
                    id="acme-enabled"
                    checked={field.value}
                    onCheckedChange={field.onChange}
                  />
                )}
              />
              <FieldLabel htmlFor="acme-enabled">
                Issue real ACME certificates
              </FieldLabel>
            </Field>
            <FieldDescription className="mt-1">
              When off (the default), every domain uses this
              platform&apos;s offline, self-signed issuer. When on, every
              routed domain gets a real certificate from Let&apos;s
              Encrypt (or the directory below), which requires the domain
              to actually resolve to this server on ports 80/443.
            </FieldDescription>

            <Controller
              control={control}
              name="acmeEnabled"
              render={({ field }) =>
                field.value ? (
                  <FieldGroup className="mt-3 gap-3">
                    <Field>
                      <FieldLabel htmlFor="acme-email">
                        Contact email
                      </FieldLabel>
                      <Input
                        id="acme-email"
                        {...register('acmeEmail')}
                        type="email"
                        placeholder="ops@example.com"
                      />
                      <FieldDescription>
                        Required by the certificate authority to reach you
                        about an issue with your certificates.
                      </FieldDescription>
                      <FieldError errors={[formState.errors.acmeEmail]} />
                    </Field>
                    <Field>
                      <FieldLabel htmlFor="acme-directory-url">
                        Directory URL (optional)
                      </FieldLabel>
                      <Input
                        id="acme-directory-url"
                        {...register('acmeDirectoryUrl')}
                        className="font-mono"
                        placeholder="https://acme-v02.api.letsencrypt.org/directory"
                      />
                      <FieldDescription>
                        Leave blank to use Let&apos;s Encrypt&apos;s
                        production directory. While testing, point this at
                        Let&apos;s Encrypt&apos;s staging directory
                        instead, to avoid production rate limits:{' '}
                        <span className="font-mono">
                          https://acme-staging-v02.api.letsencrypt.org/directory
                        </span>
                        .
                      </FieldDescription>
                    </Field>
                  </FieldGroup>
                ) : (
                  <></>
                )
              }
            />
          </div>

          <div className="flex items-center gap-2">
            <Button type="submit" size="sm" disabled={updateSettings.isPending}>
              {updateSettings.isPending ? 'Saving...' : 'Save'}
            </Button>
          </div>
          {updateSettings.isError ? (
            <Alert variant="destructive">
              <AlertDescription>
                {updateSettings.error.message}
              </AlertDescription>
            </Alert>
          ) : null}
        </form>
      </CardContent>
    </Card>
  )
}
