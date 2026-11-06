import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import commonEn from '../../locales/en/common.json'
import policyEn from '../../locales/en/storagePolicy.json'
import { TooltipProvider } from '../ui/tooltip'
import { StoragePolicyStrip } from './StoragePolicyStrip'

const state = vi.hoisted(() => ({ admin: true, saveSetting: vi.fn() }))

vi.mock('../../hooks/useAbilities', () => ({
  useAbilities: () => ({
    role: state.admin ? 'admin' : 'viewer',
    can: () => state.admin,
  }),
}))
vi.mock('../../queries/storagePolicy', () => ({
  useStoragePolicy: () => ({
    loading: false,
    janitor: {
      enabled: true,
      dry_run: true,
      disk_free_budget_percent: 15,
      build_cache_max_age_hours: 168,
    },
    settings: [
      { key: 'images.keep_per_app', value: '3' },
      { key: 'disk.low_free_mb', value: '2048' },
      { key: 'disk.critical_free_mb', value: '512' },
    ],
  }),
  useSaveStorageSetting: () => ({
    isPending: false,
    save: state.saveSetting,
  }),
  useSaveCleanupPolicy: () => ({ isPending: false, save: vi.fn() }),
}))

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['common', 'storagePolicy'],
  defaultNS: 'common',
  resources: { en: { common: commonEn, storagePolicy: policyEn } },
  interpolation: { escapeValue: false },
})

function view() {
  return render(
    <I18nextProvider i18n={i18n}>
      <TooltipProvider>
        <StoragePolicyStrip />
      </TooltipProvider>
    </I18nextProvider>,
  )
}

describe('StoragePolicyStrip', () => {
  beforeEach(() => {
    state.admin = true
    state.saveSetting.mockReset().mockResolvedValue({})
  })

  it('shows plain-word values with disk thresholds in GB', () => {
    view()
    expect(screen.getByText('On, dry run, below 15% free')).toBeInTheDocument()
    expect(screen.getByText('3 images per app')).toBeInTheDocument()
    expect(screen.getByText('7 days')).toBeInTheDocument()
    expect(screen.getAllByText('below 2 GB free')).toHaveLength(1)
    expect(screen.getByText('below 0.5 GB free')).toBeInTheDocument()
  })

  it('saves a disk threshold converted back to MB and confirms it', async () => {
    const user = userEvent.setup()
    view()
    await user.click(screen.getByRole('button', { name: /Low disk warning/ }))
    const input = screen.getByRole('textbox', { name: 'Warn below' })
    await user.clear(input)
    await user.type(input, '4')
    await user.click(screen.getByRole('button', { name: 'Save' }))
    expect(state.saveSetting).toHaveBeenCalledWith('disk.low_free_mb', 4096)
    expect(await screen.findByText('Applied')).toBeInTheDocument()
  })

  it('shows the server message when a save is rejected', async () => {
    state.saveSetting.mockRejectedValue(
      new Error('value must be between 1 and 9'),
    )
    const user = userEvent.setup()
    view()
    await user.click(screen.getByRole('button', { name: /Keep last images/ }))
    await user.click(screen.getByRole('button', { name: 'Save' }))
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'value must be between 1 and 9',
    )
  })

  it('keeps chips read-only for non-admins', async () => {
    state.admin = false
    const user = userEvent.setup()
    view()
    const chip = screen.getByRole('button', { name: /Keep last images/ })
    expect(chip).toHaveAttribute('aria-disabled', 'true')
    await user.click(chip)
    expect(screen.queryByRole('button', { name: 'Save' })).toBeNull()
  })
})
