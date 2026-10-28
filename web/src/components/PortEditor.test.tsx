import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { PortEditor } from './PortEditor'
import httpsEn from '../locales/en/https.json'
import type { AppDetail } from '../types/appDetail'

const updateMutate = vi.fn()

vi.mock('../queries/apps', () => ({
  useUpdateApp: () => ({
    mutate: updateMutate,
    isPending: false,
    isError: false,
    error: null,
  }),
}))

vi.mock('@/components/ui/toast', () => ({
  toast: { add: vi.fn() },
}))

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['https'],
  defaultNS: 'https',
  resources: { en: { https: httpsEn } },
  interpolation: { escapeValue: false },
})

const HINT = httpsEn.portEditor.publishedPortsHint

function fakeApp(): AppDetail {
  return {
    name: 'web',
    port: 3000,
    host_port: null,
    bind_address: 'private',
    published_ports: [],
  } as unknown as AppDetail
}

function renderEditor() {
  return render(
    <I18nextProvider i18n={testI18n}>
      <PortEditor app={fakeApp()} />
    </I18nextProvider>,
  )
}

describe('PortEditor extra published ports hint', () => {
  beforeEach(() => {
    updateMutate.mockReset()
  })

  it('shows the guidance hint while the field has no error', () => {
    renderEditor()
    const input = screen.getByLabelText(httpsEn.portEditor.publishedPorts)
    expect(screen.getByText(HINT)).toBeInTheDocument()
    expect(input).not.toHaveAttribute('aria-invalid', 'true')
    const describedBy = input.getAttribute('aria-describedby') ?? ''
    expect(describedBy).not.toBe('')
    const [hintId = ''] = describedBy.split(' ')
    expect(document.getElementById(hintId)).toHaveTextContent(HINT)
  })

  it('hides the hint and points aria-describedby at the error', async () => {
    const user = userEvent.setup()
    renderEditor()
    const input = screen.getByLabelText(httpsEn.portEditor.publishedPorts)
    await user.type(input, 'abc')
    await user.click(screen.getByRole('button', { name: 'Save port' }))

    const errorText = httpsEn.portEditor.errors.syntax.replace(
      '{{entry}}',
      'abc',
    )
    await waitFor(() => {
      expect(screen.getByText(errorText)).toBeInTheDocument()
    })
    expect(screen.queryByText(HINT)).not.toBeInTheDocument()
    expect(input).toHaveAttribute('aria-invalid', 'true')

    const describedBy = input.getAttribute('aria-describedby') ?? ''
    const ids = describedBy.split(' ').filter(Boolean)
    expect(ids).toHaveLength(1)
    const [errorId = ''] = ids
    expect(document.getElementById(errorId)).toHaveTextContent(errorText)
    expect(updateMutate).not.toHaveBeenCalled()
  })
})
