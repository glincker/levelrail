import { z } from 'zod'
import type { EmailSettings } from '../queries/emailSettings'

// Mirrors validateEmailSettingsRequest (internal/api/email_settings.go):
// structural fields required per provider, credentials always optional
// (blank means "leave whatever is stored alone"). Messages are
// settings:emailDelivery.validation keys, translated where rendered.
export const emailSettingsSchema = z
  .object({
    backend: z.enum([
      '',
      'smtp',
      'ses',
      'ses_smtp',
      'resend',
      'postmark',
      'sendgrid',
      'mailgun',
    ]),
    fromName: z.string().trim(),
    fromAddress: z.string().trim(),
    replyTo: z.string().trim(),
    envelopeFrom: z.string().trim(),
    sendsPerMinute: z.coerce.number().int().min(0),
    sendsPerDay: z.coerce.number().int().min(0),
    tlsMode: z.enum(['auto', 'starttls', 'tls', 'none']),
    tlsVerify: z.boolean(),
    smtpHost: z.string().trim(),
    smtpPort: z.coerce.number().int().min(0).max(65535),
    smtpUsername: z.string().trim(),
    smtpPassword: z.string(),
    sesRegion: z.string().trim(),
    sesAccessKeyId: z.string().trim(),
    sesSecretAccessKey: z.string(),
    sesSmtpPort: z.coerce.number().int(),
    sesSmtpUsername: z.string().trim(),
    sesSmtpPassword: z.string(),
    resendApiKey: z.string(),
    postmarkToken: z.string(),
    sendgridApiKey: z.string(),
    mailgunApiKey: z.string(),
    mailgunDomain: z.string().trim(),
    mailgunRegion: z.enum(['us', 'eu']),
  })
  .superRefine((data, ctx) => {
    const need = (path: keyof typeof data, message: string) =>
      ctx.addIssue({ code: 'custom', message, path: [path] })
    if (data.backend === '') return
    if (!data.fromAddress) need('fromAddress', 'validation.fromRequired')
    if (data.backend === 'smtp') {
      if (!data.smtpHost) need('smtpHost', 'validation.hostRequired')
      if (!data.smtpPort) need('smtpPort', 'validation.portRequired')
    }
    if (data.backend === 'ses') {
      if (!data.sesRegion) need('sesRegion', 'validation.regionRequired')
      if (!data.sesAccessKeyId) {
        need('sesAccessKeyId', 'validation.accessKeyRequired')
      }
    }
    if (data.backend === 'ses_smtp') {
      if (!data.sesRegion) need('sesRegion', 'validation.regionRequired')
      if (!data.sesSmtpUsername) {
        need('sesSmtpUsername', 'validation.smtpUsernameRequired')
      }
    }
    if (data.backend === 'mailgun' && !data.mailgunDomain) {
      need('mailgunDomain', 'validation.domainRequired')
    }
  })

// z.coerce.number() has a wider input type than output type, so the
// field-value type and post-validation submit type are declared separately.
export type EmailSettingsFormInput = z.input<typeof emailSettingsSchema>
export type EmailSettingsFormOutput = z.output<typeof emailSettingsSchema>

function legacyFrom(s: EmailSettings): string {
  if (s.from_address) return s.from_address
  switch (s.backend) {
    case 'smtp':
      return s.smtp_from ?? ''
    case 'ses':
    case 'ses_smtp':
      return s.ses_from ?? ''
    case 'resend':
      return s.resend_from ?? ''
    default:
      return ''
  }
}

export function toFieldValues(s: EmailSettings): EmailSettingsFormInput {
  return {
    backend: s.backend,
    fromName: s.from_name ?? '',
    fromAddress: legacyFrom(s),
    replyTo: s.reply_to ?? '',
    envelopeFrom: s.envelope_from ?? '',
    sendsPerMinute: s.sends_per_minute ?? 0,
    sendsPerDay: s.sends_per_day ?? 0,
    tlsMode: s.tls_mode || 'auto',
    tlsVerify: !s.tls_skip_verify,
    smtpHost: s.smtp_host ?? '',
    smtpPort: s.smtp_port ?? 587,
    smtpUsername: s.smtp_username ?? '',
    smtpPassword: '',
    sesRegion: s.ses_region ?? '',
    sesAccessKeyId: s.ses_access_key_id ?? '',
    sesSecretAccessKey: '',
    sesSmtpPort: s.ses_smtp_port ?? 587,
    sesSmtpUsername: s.ses_smtp_username ?? '',
    sesSmtpPassword: '',
    resendApiKey: '',
    postmarkToken: '',
    sendgridApiKey: '',
    mailgunApiKey: '',
    mailgunDomain: s.mailgun_domain ?? '',
    mailgunRegion: s.mailgun_region === 'eu' ? 'eu' : 'us',
  }
}

export function toEmailSettings(v: EmailSettingsFormOutput): EmailSettings {
  return {
    backend: v.backend,
    from_name: v.fromName,
    from_address: v.fromAddress,
    reply_to: v.replyTo,
    envelope_from: v.envelopeFrom,
    sends_per_minute: v.sendsPerMinute,
    sends_per_day: v.sendsPerDay,
    tls_mode: v.tlsMode === 'auto' ? '' : v.tlsMode,
    tls_skip_verify: !v.tlsVerify,
    smtp_host: v.smtpHost,
    smtp_port: v.smtpPort,
    smtp_username: v.smtpUsername,
    smtp_password: v.smtpPassword,
    ses_region: v.sesRegion,
    ses_access_key_id: v.sesAccessKeyId,
    ses_secret_access_key: v.sesSecretAccessKey,
    ses_smtp_port: v.sesSmtpPort,
    ses_smtp_username: v.sesSmtpUsername,
    ses_smtp_password: v.sesSmtpPassword,
    resend_api_key: v.resendApiKey,
    postmark_server_token: v.postmarkToken,
    sendgrid_api_key: v.sendgridApiKey,
    mailgun_api_key: v.mailgunApiKey,
    mailgun_domain: v.mailgunDomain,
    mailgun_region: v.mailgunRegion,
  }
}
