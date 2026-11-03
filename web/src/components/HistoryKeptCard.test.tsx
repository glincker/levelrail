import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { initReactI18next } from 'react-i18next'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import jobsEn from '../locales/en/jobs.json'
import uiSharedEn from '../locales/en/uiShared.json'
import { HistoryKeptCard } from './HistoryKeptCard'

const mutate = vi.fn()
let settings: { key: string; value: number }[] = []

vi.mock('../queries/runtimeSettings', () => ({
  useRuntimeSettings: () => ({ data: { settings } }),
  useSetRuntimeSetting: () => ({ mutate, isPending: false }),
}))

void i18next.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['jobs', 'uiShared'],
  defaultNS: 'jobs',
  resources: { en: { jobs: jobsEn, uiShared: uiSharedEn } },
  interpolation: { escapeValue: false },
})

beforeEach(() => {
  mutate.mockReset()
  settings = [{ key: 'scheduled_tasks.history_keep', value: 200 }]
})

describe('HistoryKeptCard', () => {
  it('renders nothing when the setting is unavailable', () => {
    settings = []
    const { container } = render(<HistoryKeptCard />)
    expect(container).toBeEmptyDOMElement()
  })

  it('edits and saves the value as a string', async () => {
    mutate.mockImplementation((_v, opts: { onSuccess: () => void }) => {
      opts.onSuccess()
    })
    render(<HistoryKeptCard />)
    const input = screen.getByLabelText(/History kept/)
    expect(input).toHaveValue(200)
    await userEvent.clear(input)
    await userEvent.type(input, '50')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    expect(mutate).toHaveBeenCalledWith(
      { key: 'scheduled_tasks.history_keep', value: '50' },
      expect.anything(),
    )
    await waitFor(() => {
      expect(screen.getByRole('status')).toHaveTextContent('Saved')
    })
  })

  it('blocks values outside the range', async () => {
    render(<HistoryKeptCard />)
    const input = screen.getByLabelText(/History kept/)
    await userEvent.clear(input)
    await userEvent.type(input, '0')
    expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled()
    expect(screen.getByText(/whole number from 1 to 10000/)).toBeInTheDocument()
  })
})
