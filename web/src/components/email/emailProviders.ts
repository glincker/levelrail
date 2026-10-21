import type { EmailBackend } from '../../queries/emailSettings'

export type ProviderId = Exclude<EmailBackend, ''>

export interface EmailProviderEntry {
  id: ProviderId
  /** Translation key under settings:emailDelivery.providers. */
  labelKey: ProviderId
  /** templateLogos.ts id; undefined renders the monogram tile. */
  logoId?: string
}

// One list drives the provider cards: adding a provider means one entry here and one in internal/email.
export const EMAIL_PROVIDERS: EmailProviderEntry[] = [
  { id: 'smtp', labelKey: 'smtp' },
  { id: 'ses', labelKey: 'ses', logoId: 'aws-ses' },
  { id: 'ses_smtp', labelKey: 'ses_smtp', logoId: 'aws-ses' },
  { id: 'resend', labelKey: 'resend', logoId: 'resend' },
  { id: 'postmark', labelKey: 'postmark', logoId: 'postmark' },
  { id: 'sendgrid', labelKey: 'sendgrid', logoId: 'sendgrid' },
  { id: 'mailgun', labelKey: 'mailgun', logoId: 'mailgun' },
]

export function providerById(id: EmailBackend): EmailProviderEntry | undefined {
  return EMAIL_PROVIDERS.find((p) => p.id === id)
}

// Well-known SMTP relays, prefilled on click so nobody looks up host and port by hand.
export const SMTP_PRESETS = [
  { name: 'Gmail', logoId: 'gmail', host: 'smtp.gmail.com', port: 587 },
  { name: 'Brevo', logoId: 'brevo', host: 'smtp-relay.brevo.com', port: 587 },
  {
    name: 'Mailtrap',
    logoId: 'mailtrap',
    host: 'live.smtp.mailtrap.io',
    port: 587,
  },
] as const

export const SES_SMTP_PORTS = [587, 465] as const
