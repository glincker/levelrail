import { describe, expect, it } from 'vitest'
import {
  emailSettingsSchema,
  toEmailSettings,
  toFieldValues,
} from './EmailSettingsSchema'

describe('emailSettingsSchema', () => {
  it('requires a region for SES over SMTP and the SMTP username', () => {
    const base = toFieldValues({ backend: 'ses_smtp', from_address: 'a@b.co' })
    const result = emailSettingsSchema.safeParse(base)
    expect(result.success).toBe(false)
    const paths = result.error?.issues.map((i) => i.path[0])
    expect(paths).toEqual(
      expect.arrayContaining(['sesRegion', 'sesSmtpUsername']),
    )
  })

  it('round-trips the sender and security fields', () => {
    const values = toFieldValues({
      backend: 'smtp',
      from_name: 'Ops',
      from_address: 'ops@example.com',
      smtp_host: 'smtp.example.com',
      smtp_port: 465,
      tls_mode: 'tls',
      tls_skip_verify: true,
    })
    const parsed = emailSettingsSchema.parse(values)
    const out = toEmailSettings(parsed)
    expect(out).toMatchObject({
      from_name: 'Ops',
      from_address: 'ops@example.com',
      tls_mode: 'tls',
      tls_skip_verify: true,
    })
  })

  it('falls back to the legacy per-provider from address', () => {
    expect(
      toFieldValues({ backend: 'resend', resend_from: 'old@example.com' })
        .fromAddress,
    ).toBe('old@example.com')
  })
})
