import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { AutoscaleForm } from './AutoscaleForm'
import autoscaleEn from '../../locales/en/autoscale.json'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['autoscale'],
  defaultNS: 'autoscale',
  resources: { en: { autoscale: autoscaleEn } },
  interpolation: { escapeValue: false },
})

function setup() {
  const onSave = vi.fn()
  render(
    <I18nextProvider i18n={testI18n}>
      <AutoscaleForm
        policy={null}
        pending={false}
        serverError={null}
        onSave={onSave}
        onCancel={vi.fn()}
      />
    </I18nextProvider>,
  )
  return { onSave, user: userEvent.setup() }
}

describe('AutoscaleForm', () => {
  it('starts with usable defaults and saves them', async () => {
    const { onSave, user } = setup()
    await user.click(screen.getByRole('button', { name: 'Save policy' }))
    expect(onSave).toHaveBeenCalledWith({
      min_replicas: 1,
      max_replicas: 5,
      metric: 'cpu_percent',
      target: 60,
      scale_up_tolerance_pct: 0,
      scale_down_tolerance_pct: 0,
      scale_up_cooldown_secs: 0,
      scale_down_cooldown_secs: 0,
      stabilization_secs: 0,
      max_step_up: 0,
      max_step_down: 0,
    })
  })

  it('refuses a maximum below the minimum', async () => {
    const { onSave, user } = setup()
    await user.clear(screen.getByLabelText('Minimum replicas'))
    await user.type(screen.getByLabelText('Minimum replicas'), '4')
    await user.clear(screen.getByLabelText('Maximum replicas'))
    await user.type(screen.getByLabelText('Maximum replicas'), '2')
    await user.click(screen.getByRole('button', { name: 'Save policy' }))
    expect(screen.getByRole('alert').textContent).toContain('maximum at least')
    expect(onSave).not.toHaveBeenCalled()
  })

  it('rejects a CPU target above 100 but allows it for requests', async () => {
    const { onSave, user } = setup()
    await user.clear(screen.getByLabelText(/^Target/))
    await user.type(screen.getByLabelText(/^Target/), '150')
    await user.click(screen.getByRole('button', { name: 'Save policy' }))
    expect(screen.getByRole('alert').textContent).toContain('target above zero')
    await user.click(
      screen.getByRole('radio', { name: /Requests per replica/ }),
    )
    await user.click(screen.getByRole('button', { name: 'Save policy' }))
    expect(onSave).toHaveBeenCalledWith(
      expect.objectContaining({
        metric: 'requests_per_replica',
        target: 150,
      }),
    )
  })

  it('converts minutes to seconds for the cooldowns', async () => {
    const { onSave, user } = setup()
    await user.click(screen.getByText('Advanced'))
    await user.type(screen.getByLabelText(/Wait before scaling down/), '15')
    await user.click(screen.getByRole('button', { name: 'Save policy' }))
    expect(onSave).toHaveBeenCalledWith(
      expect.objectContaining({ scale_down_cooldown_secs: 900 }),
    )
  })
})
