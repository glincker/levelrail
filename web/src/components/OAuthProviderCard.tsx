import { zodResolver } from '@hookform/resolvers/zod'
import { Controller, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'
import { CopyIcon } from '@phosphor-icons/react/dist/ssr'
import type { OAuthProviderSettings } from '../queries/oauth'
import { useUpdateOAuthProviderSettings } from '../queries/oauth'
import {
  brandKeyFor,
  callbackUrl,
  connectionState,
} from '../lib/oauthProviders'
import { BrandTile } from './BrandLogoBadge'
import { Badge } from './ui/badge'
import { Button } from './ui/button'
import { Card, CardContent, CardHeader, CardTitle } from './ui/card'
import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
} from './ui/field'
import { FormSection, StickySaveBar } from './ui/form-section'
import { Input } from './ui/input'
import { Switch } from './ui/switch'
import { toast } from './ui/toast'

interface SchemaMessages {
  clientId: string
  clientSecret: string
  issuerUrl: string
}

function buildSchema(
  provider: string,
  hasClientSecret: boolean,
  msg: SchemaMessages,
) {
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
          message: msg.clientId,
          path: ['clientId'],
        })
      }
      if (!hasClientSecret && !data.clientSecret) {
        ctx.addIssue({
          code: 'custom',
          message: msg.clientSecret,
          path: ['clientSecret'],
        })
      }
      if (provider === 'oidc' && !data.issuerUrl) {
        ctx.addIssue({
          code: 'custom',
          message: msg.issuerUrl,
          path: ['issuerUrl'],
        })
      }
    })
}

type FormValues = z.infer<ReturnType<typeof buildSchema>>

const STATE_BADGE = {
  connected: 'success',
  disabled: 'muted',
  unconfigured: 'outline',
} as const

// One card per OAuth provider: brand tile and connection state in the
// header, credentials and restrictions as two FormSections.
export function OAuthProviderCard({
  settings,
}: {
  settings: OAuthProviderSettings
}) {
  const { t } = useTranslation('settings')
  const providerLabel = (provider: string): string => {
    switch (provider) {
      case 'google':
        return t('oauthPage.providers.google')
      case 'github':
        return t('oauthPage.providers.github')
      case 'microsoft':
        return t('oauthPage.providers.microsoft')
      case 'oidc':
        return t('oauthPage.providers.oidc')
      default:
        return provider
    }
  }
  const updateSettings = useUpdateOAuthProviderSettings()
  const isOIDC = settings.provider === 'oidc'
  const label = providerLabel(settings.provider)
  const state = connectionState(settings)
  const callback = callbackUrl(window.location.origin, settings.provider)
  const id = (suffix: string) => `${settings.provider}-${suffix}`

  const { control, register, handleSubmit, reset, formState } =
    useForm<FormValues>({
      resolver: zodResolver(
        buildSchema(settings.provider, settings.has_client_secret, {
          clientId: t('oauthPage.errors.clientId'),
          clientSecret: t('oauthPage.errors.clientSecret'),
          issuerUrl: t('oauthPage.errors.issuerUrl'),
        }),
      ),
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
          toast.add({
            title: t('oauthPage.saved', { provider: label }),
            type: 'success',
          })
        },
      },
    )
  })

  const copyCallback = () => {
    void navigator.clipboard.writeText(callback).then(
      () => {
        toast.add({ title: t('oauthPage.callbackCopied'), type: 'success' })
      },
      () => {
        toast.add({ title: t('oauthPage.callbackCopyFailed'), type: 'error' })
      },
    )
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex flex-wrap items-center gap-3">
          <BrandTile
            name={brandKeyFor(settings.provider)}
            label={label}
            muted={state !== 'connected'}
          />
          <span className="text-base font-medium">{label}</span>
          <Badge variant={STATE_BADGE[state]}>
            {t(`oauthPage.state.${state}`)}
          </Badge>
          <span className="ml-auto flex items-center gap-2">
            <FieldLabel htmlFor={id('enabled')} className="text-sm font-normal">
              {t('oauthPage.enable')}
            </FieldLabel>
            <Controller
              control={control}
              name="enabled"
              render={({ field }) => (
                <Switch
                  id={id('enabled')}
                  checked={field.value}
                  onCheckedChange={field.onChange}
                />
              )}
            />
          </span>
        </CardTitle>
      </CardHeader>
      <CardContent>
        <form
          onSubmit={(e) => {
            void onSubmit(e)
          }}
          className="space-y-2"
        >
          <FormSection
            title={t('oauthPage.credentials.title')}
            description={t('oauthPage.credentials.description', {
              provider: label,
            })}
            error={
              updateSettings.isError ? updateSettings.error.message : undefined
            }
          >
            <FieldGroup className="gap-4">
              <Field>
                <FieldLabel htmlFor={id('callback')}>
                  {t('oauthPage.callbackLabel')}
                </FieldLabel>
                <div className="flex gap-2">
                  <Input
                    id={id('callback')}
                    readOnly
                    value={callback}
                    className="font-mono"
                    onFocus={(e) => {
                      e.currentTarget.select()
                    }}
                  />
                  <Button
                    type="button"
                    variant="outline"
                    size="icon"
                    aria-label={t('oauthPage.copyCallback')}
                    onClick={copyCallback}
                  >
                    <CopyIcon aria-hidden="true" />
                  </Button>
                </div>
              </Field>
              {isOIDC ? (
                <>
                  <Field>
                    <FieldLabel htmlFor={id('issuer-url')}>
                      {t('oauthPage.issuerUrl.label')}
                    </FieldLabel>
                    <Input
                      id={id('issuer-url')}
                      {...register('issuerUrl')}
                      placeholder="https://idp.example.com"
                      className="font-mono"
                    />
                    <FieldDescription>
                      {t('oauthPage.issuerUrl.hint')}
                    </FieldDescription>
                    <FieldError errors={[formState.errors.issuerUrl]} />
                  </Field>
                  <Field>
                    <FieldLabel htmlFor={id('display-name')}>
                      {t('oauthPage.displayName.label')}
                    </FieldLabel>
                    <Input
                      id={id('display-name')}
                      {...register('displayName')}
                      placeholder="Okta"
                    />
                    <FieldDescription>
                      {t('oauthPage.displayName.hint')}
                    </FieldDescription>
                  </Field>
                </>
              ) : null}
              <Field>
                <FieldLabel htmlFor={id('client-id')}>
                  {t('oauthPage.clientId')}
                </FieldLabel>
                <Input
                  id={id('client-id')}
                  {...register('clientId')}
                  className="font-mono"
                  autoComplete="off"
                />
                <FieldError errors={[formState.errors.clientId]} />
              </Field>
              <Field>
                <FieldLabel htmlFor={id('client-secret')}>
                  {t('oauthPage.clientSecret.label')}
                </FieldLabel>
                <Input
                  id={id('client-secret')}
                  type="password"
                  autoComplete="new-password"
                  {...register('clientSecret')}
                  placeholder={
                    settings.has_client_secret
                      ? t('oauthPage.clientSecret.unchanged')
                      : ''
                  }
                />
                <FieldDescription>
                  {settings.has_client_secret
                    ? t('oauthPage.clientSecret.stored')
                    : t('oauthPage.clientSecret.neverShown')}
                </FieldDescription>
                <FieldError errors={[formState.errors.clientSecret]} />
              </Field>
            </FieldGroup>
          </FormSection>

          <FormSection
            title={t('oauthPage.restrict.title')}
            description={t('oauthPage.restrict.description')}
          >
            <Field>
              <FieldLabel htmlFor={id('domain')}>
                {t('oauthPage.restrict.label')}
              </FieldLabel>
              <Input
                id={id('domain')}
                {...register('allowedEmailDomain')}
                placeholder="example.com"
              />
            </Field>
          </FormSection>

          <StickySaveBar
            dirty={formState.isDirty}
            saving={updateSettings.isPending}
            onSave={() => {
              void onSubmit()
            }}
            onReset={() => {
              reset()
            }}
          />
        </form>
      </CardContent>
    </Card>
  )
}
