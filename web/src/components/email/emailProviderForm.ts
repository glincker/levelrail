import type { UseFormReturn } from 'react-hook-form'
import { z } from 'zod'
import type { EmailProvider } from '../../queries/emailProviders'

// Messages are settings:emailDelivery.validation keys, translated where rendered.
// Credentials are always optional: blank means "leave the stored value alone".
export const emailProviderSchema = z
  .object({
    name: z.string().trim(),
    backend: z.enum([
      'smtp',
      'ses',
      'ses_smtp',
      'resend',
      'postmark',
      'sendgrid',
      'mailgun',
    ]),
    fromOverride: z.string().trim(),
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
    if (data.backend === 'smtp') {
      if (!data.smtpHost) need('smtpHost', 'validation.hostRequired')
      if (!data.smtpPort) need('smtpPort', 'validation.portRequired')
    }
    if (data.backend === 'ses' || data.backend === 'ses_smtp') {
      if (!data.sesRegion) need('sesRegion', 'validation.regionRequired')
    }
    if (data.backend === 'ses' && !data.sesAccessKeyId) {
      need('sesAccessKeyId', 'validation.accessKeyRequired')
    }
    if (data.backend === 'ses_smtp' && !data.sesSmtpUsername) {
      need('sesSmtpUsername', 'validation.smtpUsernameRequired')
    }
    if (data.backend === 'mailgun' && !data.mailgunDomain) {
      need('mailgunDomain', 'validation.domainRequired')
    }
  })

export type EmailProviderFormInput = z.input<typeof emailProviderSchema>
export type EmailProviderFormOutput = z.output<typeof emailProviderSchema>
export type EmailForm = UseFormReturn<
  EmailProviderFormInput,
  unknown,
  EmailProviderFormOutput
>
export type EmailProviderBackend = EmailProviderFormOutput['backend']

/** Backends that accept their own From address; the rest send as the shared sender. */
export const FROM_OVERRIDE_BACKENDS: ReadonlySet<EmailProviderBackend> =
  new Set(['smtp', 'ses', 'ses_smtp', 'resend'])

export const SMTP_BACKENDS: ReadonlySet<EmailProviderBackend> = new Set([
  'smtp',
  'ses_smtp',
])

function overrideOf(p: EmailProvider): string {
  switch (p.backend) {
    case 'smtp':
      return p.smtp_from ?? ''
    case 'ses':
    case 'ses_smtp':
      return p.ses_from ?? ''
    case 'resend':
      return p.resend_from ?? ''
    default:
      return ''
  }
}

/** The address this provider sends as: its own override, else the shared sender. */
export function providerFromAddress(
  p: EmailProvider,
  sharedFrom: string | undefined,
): string {
  return overrideOf(p) || sharedFrom || ''
}

export function defaultProviderValues(
  backend: EmailProviderBackend = 'smtp',
  p?: EmailProvider,
): EmailProviderFormInput {
  return {
    name: p?.name ?? '',
    backend,
    fromOverride: p && p.backend === backend ? overrideOf(p) : '',
    tlsMode: p?.tls_mode || 'auto',
    tlsVerify: !p?.tls_skip_verify,
    smtpHost: p?.smtp_host ?? '',
    smtpPort: p?.smtp_port ?? 587,
    smtpUsername: p?.smtp_username ?? '',
    smtpPassword: '',
    sesRegion: p?.ses_region ?? '',
    sesAccessKeyId: p?.ses_access_key_id ?? '',
    sesSecretAccessKey: '',
    sesSmtpPort: p?.ses_smtp_port ?? 587,
    sesSmtpUsername: p?.ses_smtp_username ?? '',
    sesSmtpPassword: '',
    resendApiKey: '',
    postmarkToken: '',
    sendgridApiKey: '',
    mailgunApiKey: '',
    mailgunDomain: p?.mailgun_domain ?? '',
    mailgunRegion: p?.mailgun_region === 'eu' ? 'eu' : 'us',
  }
}

/** toProviderPayload sends only the chosen backend's fields, so a switch never leaves stale ones behind. */
export function toProviderPayload(
  v: EmailProviderFormOutput,
  enabled: boolean | undefined,
): EmailProvider {
  const out: EmailProvider = {
    backend: v.backend,
    name: v.name,
    enabled,
  }
  switch (v.backend) {
    case 'smtp':
      Object.assign(out, {
        smtp_host: v.smtpHost,
        smtp_port: v.smtpPort,
        smtp_username: v.smtpUsername,
        smtp_password: v.smtpPassword,
        smtp_from: v.fromOverride,
        tls_mode: v.tlsMode === 'auto' ? '' : v.tlsMode,
        tls_skip_verify: !v.tlsVerify,
      })
      break
    case 'ses':
      Object.assign(out, {
        ses_region: v.sesRegion,
        ses_access_key_id: v.sesAccessKeyId,
        ses_secret_access_key: v.sesSecretAccessKey,
        ses_from: v.fromOverride,
      })
      break
    case 'ses_smtp':
      Object.assign(out, {
        ses_region: v.sesRegion,
        ses_smtp_port: v.sesSmtpPort,
        ses_smtp_username: v.sesSmtpUsername,
        ses_smtp_password: v.sesSmtpPassword,
        ses_from: v.fromOverride,
        tls_mode: v.tlsMode === 'auto' ? '' : v.tlsMode,
        tls_skip_verify: !v.tlsVerify,
      })
      break
    case 'resend':
      Object.assign(out, {
        resend_api_key: v.resendApiKey,
        resend_from: v.fromOverride,
      })
      break
    case 'postmark':
      Object.assign(out, { postmark_server_token: v.postmarkToken })
      break
    case 'sendgrid':
      Object.assign(out, { sendgrid_api_key: v.sendgridApiKey })
      break
    case 'mailgun':
      Object.assign(out, {
        mailgun_api_key: v.mailgunApiKey,
        mailgun_domain: v.mailgunDomain,
        mailgun_region: v.mailgunRegion,
      })
      break
  }
  return out
}

/** The stored provider as a write body. The list never carries credentials, so blank leaves them alone. */
export function toUnchangedPayload(
  p: EmailProvider,
  enabled: boolean,
): EmailProvider & { id: string } {
  return { ...p, id: p.id ?? '', enabled }
}
