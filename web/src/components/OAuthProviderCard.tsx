import { zodResolver } from '@hookform/resolvers/zod'
import { Controller, useForm } from 'react-hook-form'
import { z } from 'zod'
import { GithubLogoIcon, GoogleLogoIcon, ShieldCheckIcon } from '@phosphor-icons/react/dist/ssr'
import type { OAuthProviderSettings } from '../queries/oauth'
import { useUpdateOAuthProviderSettings } from '../queries/oauth'
import { Alert, AlertDescription } from './ui/alert'
import { Button } from './ui/button'
import { Card, CardContent, CardHeader, CardTitle } from './ui/card'
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from './ui/field'
import { Input } from './ui/input'
import { Switch } from './ui/switch'
import { toast } from './ui/toast'

const PROVIDER_META: Record<string, { label: string; icon: React.ReactNode }> = {
  google: { label: 'Google', icon: <GoogleLogoIcon className="size-4" /> },
  github: { label: 'GitHub', icon: <GithubLogoIcon className="size-4" /> },
  oidc: { label: 'OIDC / SSO', icon: <ShieldCheckIcon className="size-4" /> },
}

function buildSchema(provider: string, hasClientSecret: boolean) {
  return z
    .object({
      enabled: z.boolean(),
      clientId: z.string().trim(),
      clientSecret: z.string(),
      allowedEmailDomain: z.string().trim(),
      issuerUrl: z.string().trim(),
      displayName: z.string().trim(),
    })
    .superRefine((data, ctx) => {
      if (!data.enabled) {
        return
      }
      if (!data.clientId) {
        ctx.addIssue({
          code: 'custom',
          message: 'Client ID is required to enable this provider',
          path: ['clientId'],
        })
      }
      if (!hasClientSecret && !data.clientSecret) {
        ctx.addIssue({
          code: 'custom',
          message: 'Client secret is required the first time this provider is enabled',
          path: ['clientSecret'],
        })
      }
      if (provider === 'oidc' && !data.issuerUrl) {
        ctx.addIssue({
          code: 'custom',
          message: 'Issuer URL is required to enable OIDC',
          path: ['issuerUrl'],
        })
      }
    })
}

type FormValues = z.infer<ReturnType<typeof buildSchema>>

// One card per OAuth provider: enabled toggle, client ID, client secret
// (write-only, never redisplayed), and an optional email domain allowlist.
export function OAuthProviderCard({
  settings,
}: {
  settings: OAuthProviderSettings
}) {
  const updateSettings = useUpdateOAuthProviderSettings()
  const meta = PROVIDER_META[settings.provider]
  const isOIDC = settings.provider === 'oidc'
  const { control, register, handleSubmit, formState } = useForm<FormValues>({
    resolver: zodResolver(buildSchema(settings.provider, settings.has_client_secret)),
    values: {
      enabled: settings.enabled,
      clientId: settings.client_id ?? '',
      clientSecret: '',
      allowedEmailDomain: settings.allowed_email_domain ?? '',
      issuerUrl: settings.issuer_url ?? '',
      displayName: settings.display_name ?? '',
    },
    resetOptions: { keepDirtyValues: true },
  })

  const onSubmit = handleSubmit((values) => {
    updateSettings.mutate(
      {
        provider: settings.provider,
        req: {
          enabled: values.enabled,
          client_id: values.clientId,
          client_secret: values.clientSecret || undefined,
          allowed_email_domain: values.allowedEmailDomain,
          issuer_url: values.issuerUrl,
          display_name: values.displayName,
        },
      },
      {
        onSuccess: () => {
          toast.add({ title: `${meta?.label ?? settings.provider} settings saved.`, type: 'success' })
        },
      },
    )
  })

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          {meta?.icon}
          {meta?.label ?? settings.provider}
        </CardTitle>
      </CardHeader>
      <CardContent>
        <form
          onSubmit={(e) => {
            void onSubmit(e)
          }}
          className="space-y-4"
        >
          <Field orientation="horizontal">
            <Controller
              control={control}
              name="enabled"
              render={({ field }) => (
                <Switch
                  id={`${settings.provider}-enabled`}
                  checked={field.value}
                  onCheckedChange={field.onChange}
                />
              )}
            />
            <FieldLabel htmlFor={`${settings.provider}-enabled`}>
              Allow sign-in with {meta?.label ?? settings.provider}
            </FieldLabel>
          </Field>

          <FieldGroup className="gap-3">
            {isOIDC ? (
              <Field>
                <FieldLabel htmlFor={`${settings.provider}-issuer-url`}>
                  Issuer URL
                </FieldLabel>
                <Input
                  id={`${settings.provider}-issuer-url`}
                  {...register('issuerUrl')}
                  placeholder="https://idp.example.com"
                  className="font-mono"
                />
                <FieldDescription>
                  Your identity provider's OIDC issuer, e.g. what
                  /.well-known/openid-configuration is served from.
                </FieldDescription>
                <FieldError errors={[formState.errors.issuerUrl]} />
              </Field>
            ) : null}
            {isOIDC ? (
              <Field>
                <FieldLabel htmlFor={`${settings.provider}-display-name`}>
                  Display name (optional)
                </FieldLabel>
                <Input
                  id={`${settings.provider}-display-name`}
                  {...register('displayName')}
                  placeholder="Okta"
                />
                <FieldDescription>
                  Shown on the login button as "Continue with ...". Defaults
                  to "SSO".
                </FieldDescription>
              </Field>
            ) : null}
            <Field>
              <FieldLabel htmlFor={`${settings.provider}-client-id`}>
                Client ID
              </FieldLabel>
              <Input
                id={`${settings.provider}-client-id`}
                {...register('clientId')}
                className="font-mono"
              />
              <FieldError errors={[formState.errors.clientId]} />
            </Field>
            <Field>
              <FieldLabel htmlFor={`${settings.provider}-client-secret`}>
                Client secret
              </FieldLabel>
              <Input
                id={`${settings.provider}-client-secret`}
                type="password"
                {...register('clientSecret')}
                placeholder={settings.has_client_secret ? 'Unchanged' : ''}
              />
              <FieldDescription>
                {settings.has_client_secret
                  ? 'A secret is already stored. Leave blank to keep it.'
                  : 'Never shown again once saved.'}
              </FieldDescription>
              <FieldError errors={[formState.errors.clientSecret]} />
            </Field>
            <Field>
              <FieldLabel htmlFor={`${settings.provider}-domain`}>
                Restrict to email domain (optional)
              </FieldLabel>
              <Input
                id={`${settings.provider}-domain`}
                {...register('allowedEmailDomain')}
                placeholder="example.com"
              />
              <FieldDescription>
                Only new sign-ins from this email domain get an account
                created automatically. Leave blank to allow any email.
              </FieldDescription>
            </Field>
          </FieldGroup>

          <Button type="submit" size="sm" disabled={updateSettings.isPending}>
            {updateSettings.isPending ? 'Saving...' : 'Save'}
          </Button>
          {updateSettings.isError ? (
            <Alert variant="destructive">
              <AlertDescription>{updateSettings.error.message}</AlertDescription>
            </Alert>
          ) : null}
        </form>
      </CardContent>
    </Card>
  )
}
