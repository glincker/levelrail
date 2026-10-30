import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { describe, expect, it, vi } from 'vitest'
import smartDeployEn from '../../locales/en/smartDeploy.json'
import { ScheduleDeployField } from './ScheduleDeployField'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['smartDeploy'],
  defaultNS: 'smartDeploy',
  resources: { en: { smartDeploy: smartDeployEn } },
  interpolation: { escapeValue: false },
})

function renderField(
  value: { enabled: boolean; when: string },
  error: 'required' | 'invalid' | 'past' | null,
  onChange = vi.fn(),
) {
  render(
    <I18nextProvider i18n={testI18n}>
      <ScheduleDeployField value={value} error={error} onChange={onChange} />
    </I18nextProvider>,
  )
  return onChange
}

describe('ScheduleDeployField', () => {
  it('hides the picker until scheduling is turned on', async () => {
    const onChange = renderField({ enabled: false, when: '' }, null)
    expect(screen.queryByLabelText('Deploy at')).toBeNull()
    await userEvent.click(screen.getByRole('checkbox'))
    expect(onChange).toHaveBeenCalledWith({ enabled: true, when: '' })
  })

  it('shows the picker with the timezone when on', () => {
    renderField({ enabled: true, when: '2027-01-01T03:00' }, null)
    expect(screen.getByLabelText('Deploy at')).toHaveValue('2027-01-01T03:00')
    expect(screen.getByText(/Times are in your timezone/)).toBeTruthy()
  })

  it.each([
    ['required', 'Pick a date and time.'],
    ['invalid', 'That date and time is not valid.'],
    ['past', 'Pick a time in the future.'],
  ] as const)('shows the %s error', (error, message) => {
    renderField({ enabled: true, when: '' }, error)
    expect(screen.getByRole('alert')).toHaveTextContent(message)
    expect(screen.getByLabelText('Deploy at')).toHaveAttribute(
      'aria-invalid',
      'true',
    )
  })
})
