import { zodResolver } from '@hookform/resolvers/zod'
import { Controller, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'
import { CopyGlassIcon } from '@thesvg/ui-react'
import type { OAuthProviderSettings } from '../queries/oauth'
import { useUpdateOAuthProviderSettings } from '../queries/oauth'
import { callbackUrl } from '../lib/oauthProviders'
import { Button } from './ui/button'
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
  instanceUrl: string
  userPoolId: string
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
      if (
        provider === 'gitlab' &&
        data.issuerUrl &&
        !/^https:\/\//i.test(data.issuerUrl)
      ) {
        ctx.addIssue({
          code: 'custom',
          message: msg.instanceUrl,
          path: ['issuerUrl'],
        })
      }
      if (
        provider === 'cognito' &&
        !/^[a-z]{2}(-[a-z]+)+-\d_[A-Za-z0-9]+$/.test(data.issuerUrl)
      ) {
        ctx.addIssue({
          code: 'custom',
          message: msg.userPoolId,
          path: ['issuerUrl'],
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

// The editable form for one OAuth provider, shown inside the provider's
// settings sheet: enable switch, credentials and restrictions sections.
export function OAuthProviderForm({
  settings,
  label,
}: {
  settings: OAuthProviderSettings
  label: string
}) {
  const { t } = useTranslation('settings')
  const updateSettings = useUpdateOAuthProviderSettings()
  const isOIDC = settings.provider === 'oidc'
  const isGitLab = settings.provider === 'gitlab'
  const isCognito = settings.provider === 'cognito'
  const callback = callbackUrl(window.location.origin, settings.provider)
  const id = (suffix: string) => `${settings.provider}-${suffix}`

  const { control, register, handleSubmit, reset, formState } =
    useForm<FormValues>({
      resolver: zodResolver(
        buildSchema(settings.provider, settings.has_client_secret, {
          clientId: t('oauthPage.errors.clientId'),
          clientSecret: t('oauthPage.errors.clientSecret'),
          issuerUrl: t('oauthPage.errors.issuerUrl'),
          instanceUrl: t('oauthPage.instanceUrl.invalid'),
          userPoolId: t('oauthPage.userPoolId.invalid'),
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
    <form
      onSubmit={(e) => {
        void onSubmit(e)
      }}
      className="space-y-2"
    >
      <div className="flex items-center justify-between gap-3 pb-2">
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
      </div>
      <FormSection
        className="lg:grid-cols-1"
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
                <CopyGlassIcon size="1.2em" aria-hidden="true" />
              </Button>
            </div>
          </Field>
          {isGitLab ? (
            <Field>
              <FieldLabel htmlFor={id('issuer-url')}>
                {t('oauthPage.instanceUrl.label')}
              </FieldLabel>
              <Input
                id={id('issuer-url')}
                {...register('issuerUrl')}
                placeholder="https://gitlab.example.com"
                className="font-mono"
              />
              <FieldDescription>
                {t('oauthPage.instanceUrl.hint')}
              </FieldDescription>
              <FieldError errors={[formState.errors.issuerUrl]} />
            </Field>
          ) : null}
          {isCognito ? (
            <Field>
              <FieldLabel htmlFor={id('issuer-url')}>
                {t('oauthPage.userPoolId.label')}
              </FieldLabel>
              <Input
                id={id('issuer-url')}
                {...register('issuerUrl')}
                placeholder="us-east-1_AbC123"
                className="font-mono"
              />
              <FieldDescription>
                {t('oauthPage.userPoolId.hint')}
              </FieldDescription>
              <FieldError errors={[formState.errors.issuerUrl]} />
            </Field>
          ) : null}
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
        className="lg:grid-cols-1"
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
  )
}
