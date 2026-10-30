import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { createInstance } from 'i18next'
import common from '../../locales/en/common.json'
import { AppPowerConfirm } from './AppPowerConfirm'

async function open(kind: 'restart' | 'stop') {
  const i18n = createInstance()
  await i18n
    .use(initReactI18next)
    .init({ lng: 'en', defaultNS: 'common', resources: { en: { common } } })
  render(
    <I18nextProvider i18n={i18n}>
      <AppPowerConfirm
        kind={kind}
        name="web"
        pending={false}
        onCancel={vi.fn()}
        onConfirm={vi.fn()}
      />
    </I18nextProvider>,
  )
}

describe('AppPowerConfirm', () => {
  it.each(['restart', 'stop'] as const)(
    'focuses Cancel first on %s',
    async (kind) => {
      await open(kind)
      const cancel = await screen.findByRole('button', { name: 'Cancel' })
      await vi.waitFor(() => {
        expect(document.activeElement).toBe(cancel)
      })
    },
  )

  it('makes the stop confirm a solid destructive button', async () => {
    await open('stop')
    const confirm = await screen.findByRole('button', {
      name: common.appPower.stopConfirm,
    })
    expect(confirm.className).toContain('bg-destructive')
    expect(confirm.className).not.toContain('bg-destructive/10')
  })
})
