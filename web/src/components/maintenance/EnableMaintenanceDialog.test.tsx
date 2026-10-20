import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import maintenanceEn from '../../locales/en/maintenance.json'
import { EnableMaintenanceDialog } from './EnableMaintenanceDialog'

vi.mock('./MaintenancePreview', () => ({
  MaintenancePreview: () => <div>preview frame</div>,
}))

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['maintenance', 'common'],
  defaultNS: 'maintenance',
  resources: { en: { maintenance: maintenanceEn, common: {} } },
  interpolation: { escapeValue: false },
})

function renderDialog(
  request: Parameters<typeof EnableMaintenanceDialog>[0]['request'],
  onConfirm = vi.fn(),
) {
  render(
    <I18nextProvider i18n={i18n}>
      <EnableMaintenanceDialog
        domain="app.test"
        request={request}
        defaultMaxSeconds={7200}
        pending={false}
        onCancel={vi.fn()}
        onConfirm={onConfirm}
      />
    </I18nextProvider>,
  )
  return onConfirm
}

describe('EnableMaintenanceDialog', () => {
  it('renders nothing until a request is pending confirmation', () => {
    renderDialog(null)
    expect(screen.queryByText(/Put app.test into maintenance/)).toBeNull()
  })

  it('states what visitors get, that assets are not served, and the default auto-off', async () => {
    const onConfirm = renderDialog({})
    expect(
      await screen.findByText('Put app.test into maintenance?'),
    ).toBeVisible()
    expect(screen.getByText(/Scripts, styles, images/)).toBeVisible()
    expect(screen.getByText(/maintenance stays on/)).toBeVisible()
    expect(screen.getByText(/after 2 hours/)).toBeVisible()
    expect(screen.getByText('preview frame')).toBeVisible()
    await userEvent.click(
      screen.getByRole('button', { name: 'Turn maintenance on' }),
    )
    expect(onConfirm).toHaveBeenCalledWith({})
  })

  it('warns when nothing will end maintenance', async () => {
    renderDialog({ max_duration_seconds: 0 })
    expect(await screen.findByText(/no automatic end/)).toBeVisible()
  })
})
