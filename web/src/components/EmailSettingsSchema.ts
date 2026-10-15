import { z } from 'zod'
import type { EmailSettings } from '../queries/emailSettings'

// Mirrors validateEmailSettingsRequest (internal/api/email_settings.go):
// structural fields required per backend, credentials always optional
// (blank means "leave whatever is stored alone").
export const emailSettingsSchema = z
  .object({
    backend: z.enum(['', 'smtp', 'ses', 'resend']),
    smtpHost: z.string().trim(),
    smtpPort: z.coerce.number().int().min(0).max(65535),
    smtpUsername: z.string().trim(),
    smtpFrom: z.string().trim(),
    smtpPassword: z.string(),
    sesRegion: z.string().trim(),
    sesAccessKeyId: z.string().trim(),
    sesFrom: z.string().trim(),
    sesSecretAccessKey: z.string(),
    resendFrom: z.string().trim(),
    resendApiKey: z.string(),
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
    if (data.backend === 'resend' && !data.resendFrom) {
      ctx.addIssue({
        code: 'custom',
        message: 'From address is required',
        path: ['resendFrom'],
      })
    }
  })

// z.coerce.number() (smtpPort) has a wider input type than output type,
// so the field-value type and post-validation submit type are declared
// separately, the same split CreateAlertRuleDialog's own comment
// explains for its threshold field.
export type EmailSettingsFormInput = z.input<typeof emailSettingsSchema>
export type EmailSettingsFormOutput = z.output<typeof emailSettingsSchema>

export function toFieldValues(s: EmailSettings): EmailSettingsFormInput {
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
    resendFrom: s.resend_from ?? '',
    resendApiKey: '',
  }
}

export function toEmailSettings(v: EmailSettingsFormOutput): EmailSettings {
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
    resend_from: v.resendFrom,
    resend_api_key: v.resendApiKey,
  }
}
