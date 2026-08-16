import { zodResolver } from '@hookform/resolvers/zod'
import { Controller, useForm } from 'react-hook-form'
import { z } from 'zod'
import { EnvelopeIcon } from '@phosphor-icons/react/dist/ssr'
import type { EmailSettings } from '../queries/emailSettings'
import { useUpdateEmailSettings } from '../queries/emailSettings'
import { Alert, AlertDescription } from '@/components/ui/alert'
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { toast } from '@/components/ui/toast'

// Sentinel for the Select's "no backend chosen" option: an empty-string
// SelectItem value is unreliable across select implementations, so this
// maps to/from backend === '' at the Controller boundary instead.
const BACKEND_UNSET = 'unset'

// Mirrors validateEmailSettingsRequest (internal/api/email_settings.go):
// structural fields required per backend, credentials always optional
// (blank means "leave whatever is stored alone").
const emailSettingsSchema = z
  .object({
    backend: z.enum(['', 'smtp', 'ses']),
    smtpHost: z.string().trim(),
    smtpPort: z.coerce.number().int().min(0).max(65535),
    smtpUsername: z.string().trim(),
    smtpFrom: z.string().trim(),
    smtpPassword: z.string(),
    sesRegion: z.string().trim(),
    sesAccessKeyId: z.string().trim(),
    sesFrom: z.string().trim(),
    sesSecretAccessKey: z.string(),
  })
  .superRefine((data, ctx) => {
    if (data.backend === 'smtp') {
      if (!data.smtpHost) {
        ctx.addIssue({
          code: 'custom',
          message: 'Host is required',
          path: ['smtpHost'],
        })
      }
      if (!data.smtpFrom) {
        ctx.addIssue({
          code: 'custom',
          message: 'From address is required',
          path: ['smtpFrom'],
        })
      }
      if (!data.smtpPort) {
        ctx.addIssue({
          code: 'custom',
          message: 'Port is required',
          path: ['smtpPort'],
        })
      }
    }
    if (data.backend === 'ses') {
      if (!data.sesRegion) {
        ctx.addIssue({
          code: 'custom',
          message: 'Region is required',
          path: ['sesRegion'],
        })
      }
      if (!data.sesFrom) {
        ctx.addIssue({
          code: 'custom',
          message: 'From address is required',
          path: ['sesFrom'],
        })
      }
      if (!data.sesAccessKeyId) {
        ctx.addIssue({
          code: 'custom',
          message: 'Access key ID is required',
          path: ['sesAccessKeyId'],
        })
      }
    }
  })

// z.coerce.number() (smtpPort) has a wider input type than output type,
// so the field-value type and post-validation submit type are declared
// separately, the same split CreateAlertRuleDialog's own comment
// explains for its threshold field.
type EmailSettingsFormInput = z.input<typeof emailSettingsSchema>
type EmailSettingsFormOutput = z.output<typeof emailSettingsSchema>

function toFieldValues(s: EmailSettings): EmailSettingsFormInput {
  return {
    backend: s.backend,
    smtpHost: s.smtp_host ?? '',
    smtpPort: s.smtp_port ?? 0,
    smtpUsername: s.smtp_username ?? '',
    smtpFrom: s.smtp_from ?? '',
    smtpPassword: '',
    sesRegion: s.ses_region ?? '',
    sesAccessKeyId: s.ses_access_key_id ?? '',
    sesFrom: s.ses_from ?? '',
    sesSecretAccessKey: '',
  }
}

function toEmailSettings(v: EmailSettingsFormOutput): EmailSettings {
  return {
    backend: v.backend,
    smtp_host: v.smtpHost,
    smtp_port: v.smtpPort,
    smtp_username: v.smtpUsername,
    smtp_from: v.smtpFrom,
    smtp_password: v.smtpPassword,
    ses_region: v.sesRegion,
    ses_access_key_id: v.sesAccessKeyId,
    ses_from: v.sesFrom,
    ses_secret_access_key: v.sesSecretAccessKey,
  }
}

// Platform-wide email backend (SMTP or AWS SES): GET/PUT
// /api/v1/settings/email. Used by both internal/alerting's
// notifications and the forgot-password flow.
export function EmailSettingsCard({ settings }: { settings: EmailSettings }) {
  const updateSettings = useUpdateEmailSettings()
  const { control, register, handleSubmit, formState } = useForm<
    EmailSettingsFormInput,
    unknown,
    EmailSettingsFormOutput
  >({
    resolver: zodResolver(emailSettingsSchema),
    values: toFieldValues(settings),
    resetOptions: { keepDirtyValues: true },
  })

  const onSubmit = handleSubmit((values) => {
    updateSettings.mutate(toEmailSettings(values), {
      onSuccess: () => {
        toast.add({ title: 'Email settings saved.', type: 'success' })
      },
    })
  })

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <EnvelopeIcon className="size-4" />
          Email
        </CardTitle>
        <CardDescription>
          The backend alert notifications and password-reset emails are sent
          through. Off by default; a control plane started with APP_SMTP_* env
          vars still works until this is saved.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form
          onSubmit={(e) => {
            void onSubmit(e)
          }}
          className="space-y-5"
        >
          <Field>
            <FieldLabel htmlFor="email-backend">Backend</FieldLabel>
            <Controller
              control={control}
              name="backend"
              render={({ field }) => (
                <Select
                  value={field.value || BACKEND_UNSET}
                  onValueChange={(v) =>
                    field.onChange(v === BACKEND_UNSET ? '' : v)
                  }
                >
                  <SelectTrigger id="email-backend" className="w-full sm:w-64">
                    <SelectValue placeholder="Not configured" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value={BACKEND_UNSET}>
                      Not configured
                    </SelectItem>
                    <SelectItem value="smtp">SMTP</SelectItem>
                    <SelectItem value="ses">AWS SES</SelectItem>
                  </SelectContent>
                </Select>
              )}
            />
          </Field>

          <Controller
            control={control}
            name="backend"
            render={({ field }) =>
              field.value === 'smtp' ? (
                <FieldGroup className="gap-3 rounded-md border border-border p-3">
                  <Field>
                    <FieldLabel htmlFor="smtp-host">Host</FieldLabel>
                    <Input
                      id="smtp-host"
                      {...register('smtpHost')}
                      placeholder="smtp.example.com"
                    />
                    <FieldError errors={[formState.errors.smtpHost]} />
                  </Field>
                  <Field>
                    <FieldLabel htmlFor="smtp-port">Port</FieldLabel>
                    <Input
                      id="smtp-port"
                      type="number"
                      {...register('smtpPort')}
                      placeholder="587"
                    />
                    <FieldError errors={[formState.errors.smtpPort]} />
                  </Field>
                  <Field>
                    <FieldLabel htmlFor="smtp-username">Username</FieldLabel>
                    <Input id="smtp-username" {...register('smtpUsername')} />
                  </Field>
                  <Field>
                    <FieldLabel htmlFor="smtp-from">From address</FieldLabel>
                    <Input
                      id="smtp-from"
                      {...register('smtpFrom')}
                      placeholder="no-reply@example.com"
                    />
                    <FieldError errors={[formState.errors.smtpFrom]} />
                  </Field>
                  <Field>
                    <FieldLabel htmlFor="smtp-password">Password</FieldLabel>
                    <Input
                      id="smtp-password"
                      type="password"
                      {...register('smtpPassword')}
                    />
                    <FieldDescription>
                      {settings.smtp_password_set
                        ? 'A password is already configured. Leave blank to keep it.'
                        : 'No password set yet.'}
                    </FieldDescription>
                  </Field>
                </FieldGroup>
              ) : field.value === 'ses' ? (
                <FieldGroup className="gap-3 rounded-md border border-border p-3">
                  <Field>
                    <FieldLabel htmlFor="ses-region">Region</FieldLabel>
                    <Input
                      id="ses-region"
                      {...register('sesRegion')}
                      placeholder="us-east-1"
                    />
                    <FieldError errors={[formState.errors.sesRegion]} />
                  </Field>
                  <Field>
                    <FieldLabel htmlFor="ses-access-key-id">
                      Access key ID
                    </FieldLabel>
                    <Input
                      id="ses-access-key-id"
                      {...register('sesAccessKeyId')}
                    />
                    <FieldError errors={[formState.errors.sesAccessKeyId]} />
                  </Field>
                  <Field>
                    <FieldLabel htmlFor="ses-from">From address</FieldLabel>
                    <Input
                      id="ses-from"
                      {...register('sesFrom')}
                      placeholder="no-reply@example.com"
                    />
                    <FieldError errors={[formState.errors.sesFrom]} />
                  </Field>
                  <Field>
                    <FieldLabel htmlFor="ses-secret-access-key">
                      Secret access key
                    </FieldLabel>
                    <Input
                      id="ses-secret-access-key"
                      type="password"
                      {...register('sesSecretAccessKey')}
                    />
                    <FieldDescription>
                      {settings.ses_secret_access_key_set
                        ? 'A secret key is already configured. Leave blank to keep it.'
                        : 'No secret key set yet.'}
                    </FieldDescription>
                  </Field>
                </FieldGroup>
              ) : (
                <></>
              )
            }
          />

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
