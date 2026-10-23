import { describe, expect, it } from 'vitest'
import {
  ACCEPTED_BACKENDS,
  EMAIL_PROVIDER_PRESETS,
  presetById,
} from './emailProviderPresets'
import { defaultProviderValues, emailProviderSchema } from './emailProviderForm'
import settingsEn from '../../locales/en/settings.json'

describe('email provider presets', () => {
  it('maps every preset to a backend the API accepts', () => {
    for (const p of EMAIL_PROVIDER_PRESETS) {
      expect(ACCEPTED_BACKENDS).toContain(p.backend)
      expect(p.defaults.backend).toBe(p.backend)
    }
  })

  it('has unique ids and copy for each', () => {
    const ids = EMAIL_PROVIDER_PRESETS.map((p) => p.id)
    expect(new Set(ids).size).toBe(ids.length)
    for (const id of ids) {
      const item = settingsEn.emailPresets.items[id]
      expect(item.name).toBeTruthy()
      expect(item.bestFor).toBeTruthy()
      expect(item.credentialHint).toBeTruthy()
    }
  })

  it('prefills a port for every SMTP preset and a host for named relays', () => {
    for (const p of EMAIL_PROVIDER_PRESETS.filter(
      (x) => x.backend === 'smtp',
    )) {
      expect(p.defaults.smtpPort).toBeGreaterThan(0)
      if (p.id !== 'smtp') expect(p.defaults.smtpHost).toMatch(/\./)
    }
  })

  it('never carries a credential or sender, and docs links are https', () => {
    for (const p of EMAIL_PROVIDER_PRESETS) {
      const keys = Object.keys(p.defaults)
      for (const secret of [
        'smtpPassword',
        'sesSecretAccessKey',
        'sesSmtpPassword',
        'resendApiKey',
        'postmarkToken',
        'sendgridApiKey',
        'mailgunApiKey',
        'fromOverride',
      ]) {
        expect(keys).not.toContain(secret)
      }
      if (p.docsUrl) expect(p.docsUrl.startsWith('https://')).toBe(true)
    }
  })

  it('applies onto the provider form so a relay validates with only the credential missing', () => {
    const gmail = presetById('gmail')
    const values = {
      ...defaultProviderValues(gmail?.backend),
      ...gmail?.defaults,
    }
    expect(values.smtpHost).toBe('smtp.gmail.com')
    expect(values.tlsMode).toBe('starttls')
    expect(emailProviderSchema.safeParse(values).success).toBe(true)
  })
})
