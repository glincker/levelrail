import type {
  EmailProviderBackend,
  EmailProviderFormInput,
} from './emailProviderForm'

// Backends internal/api/email_settings.go accepts; a preset must map to one of these.
export const ACCEPTED_BACKENDS: readonly EmailProviderBackend[] = [
  'smtp',
  'ses',
  'ses_smtp',
  'resend',
  'postmark',
  'sendgrid',
  'mailgun',
]

export type PresetId =
  | 'resend'
  | 'ses'
  | 'ses_smtp'
  | 'postmark'
  | 'sendgrid'
  | 'mailgun'
  | 'gmail'
  | 'microsoft365'
  | 'brevo'
  | 'smtp'

export interface EmailProviderPreset {
  id: PresetId
  backend: EmailProviderBackend
  /** templateLogos.ts id; undefined renders the monogram tile. */
  logoId?: string
  /** Only non-secret form values: credentials and the sender are always the user's. */
  defaults: Partial<EmailProviderFormInput>
  docsUrl?: string
}

// Names, hints and "best for" lines live in settings:emailPresets.items.<id>.
// Values here are connection facts the vendor publishes; anything unverified is left out.
export const EMAIL_PROVIDER_PRESETS: readonly EmailProviderPreset[] = [
  {
    id: 'resend',
    backend: 'resend',
    logoId: 'resend',
    defaults: { backend: 'resend', name: 'Resend' },
    docsUrl: 'https://resend.com/docs/dashboard/api-keys/introduction',
  },
  {
    id: 'ses',
    backend: 'ses',
    logoId: 'aws-ses',
    defaults: { backend: 'ses', name: 'Amazon SES' },
    docsUrl: 'https://docs.aws.amazon.com/ses/latest/dg/send-email.html',
  },
  {
    id: 'ses_smtp',
    backend: 'ses_smtp',
    logoId: 'aws-ses',
    defaults: {
      backend: 'ses_smtp',
      name: 'Amazon SES SMTP',
      sesSmtpPort: 587,
      tlsMode: 'starttls',
    },
    docsUrl: 'https://docs.aws.amazon.com/ses/latest/dg/smtp-credentials.html',
  },
  {
    id: 'postmark',
    backend: 'postmark',
    logoId: 'postmark',
    defaults: { backend: 'postmark', name: 'Postmark' },
    docsUrl: 'https://postmarkapp.com/developer/api/overview',
  },
  {
    id: 'sendgrid',
    backend: 'sendgrid',
    logoId: 'sendgrid',
    defaults: { backend: 'sendgrid', name: 'SendGrid' },
    docsUrl:
      'https://www.twilio.com/docs/sendgrid/ui/account-and-settings/api-keys',
  },
  {
    id: 'mailgun',
    backend: 'mailgun',
    logoId: 'mailgun',
    defaults: { backend: 'mailgun', name: 'Mailgun', mailgunRegion: 'us' },
    docsUrl: 'https://documentation.mailgun.com/',
  },
  {
    id: 'gmail',
    backend: 'smtp',
    logoId: 'gmail',
    defaults: {
      backend: 'smtp',
      name: 'Google Workspace',
      smtpHost: 'smtp.gmail.com',
      smtpPort: 587,
      tlsMode: 'starttls',
    },
    docsUrl: 'https://support.google.com/accounts/answer/185833',
  },
  {
    id: 'microsoft365',
    backend: 'smtp',
    defaults: {
      backend: 'smtp',
      name: 'Microsoft 365',
      smtpHost: 'smtp.office365.com',
      smtpPort: 587,
      tlsMode: 'starttls',
    },
    docsUrl:
      'https://learn.microsoft.com/en-us/exchange/mail-flow-best-practices/how-to-set-up-a-multifunction-device-or-application-to-send-email-using-microsoft-365-or-office-365',
  },
  {
    id: 'brevo',
    backend: 'smtp',
    logoId: 'brevo',
    defaults: {
      backend: 'smtp',
      name: 'Brevo',
      smtpHost: 'smtp-relay.brevo.com',
      smtpPort: 587,
      tlsMode: 'starttls',
    },
    docsUrl: 'https://help.brevo.com/hc/en-us/articles/7959631848850',
  },
  {
    id: 'smtp',
    backend: 'smtp',
    defaults: { backend: 'smtp', smtpPort: 587 },
  },
]

export function presetById(id: PresetId): EmailProviderPreset | undefined {
  return EMAIL_PROVIDER_PRESETS.find((p) => p.id === id)
}
