import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { CheckDetail } from './CheckDetail'
import fixitEn from '../locales/en/fixit.json'
import commonEn from '../locales/en/common.json'
import type { DoctorCheck } from '../queries/systemDoctor'

vi.mock('../queries/fixit', () => ({ useFixForCheck: () => undefined }))
vi.mock('./doctorCheckCtas', () => ({ getCheckCta: () => undefined }))
vi.mock('./ApiErrorAlert', () => ({
  ApiErrorAlert: () => <div role="alert">boom</div>,
}))

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['fixit', 'common'],
  defaultNS: 'fixit',
  resources: { en: { fixit: fixitEn, common: commonEn } },
  interpolation: { escapeValue: false },
})

const check: DoctorCheck = {
  code: 'disk_space',
  name: 'Disk space',
  status: 'ok',
  message: 'Plenty of space',
}

function renderDetail() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <I18nextProvider i18n={testI18n}>
        <CheckDetail check={check} />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('CheckDetail explain', () => {
  it('does not fetch until opened, then shows the explanation', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          id: 'disk_space',
          plain: 'The disk is nearly full.',
          why: 'Builds fail.',
          next: 'Open Fixes.',
          fix_id: 'disk.reclaim',
          kind: 'auto',
        }),
        { status: 200 },
      ),
    )
    vi.stubGlobal('fetch', fetchMock)
    renderDetail()
    expect(fetchMock).not.toHaveBeenCalled()
    await userEvent.click(screen.getByRole('button', { name: 'Explain' }))
    expect(await screen.findByText('The disk is nearly full.')).toBeTruthy()
    expect(screen.getByText('disk.reclaim (auto)')).toBeTruthy()
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('shows the error alert when the request fails', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(new Response('{}', { status: 404 })),
    )
    renderDetail()
    await userEvent.click(screen.getByRole('button', { name: 'Explain' }))
    expect(await screen.findByRole('alert')).toBeTruthy()
  })
})
