import { fireEvent, render, screen } from '@testing-library/react'
import i18next from 'i18next'
import { initReactI18next } from 'react-i18next'
import { describe, expect, it, vi } from 'vitest'
import commonEn from '../../locales/en/common.json'
import settingsEn from '../../locales/en/settings.json'
import { InfrastructurePage } from './InfrastructurePage'

void i18next.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['common', 'settings'],
  defaultNS: 'common',
  resources: { en: { common: commonEn, settings: settingsEn } },
  interpolation: { escapeValue: false },
})

const planState = vi.hoisted(() => ({ error: null as Error | null }))
const planMutate = vi.hoisted(() => vi.fn())

vi.mock('../../queries/iac', () => ({
  IacIssuesError: class extends Error {
    issues: unknown[] = []
  },
  usePlanIac: () => ({
    mutate: planMutate,
    isPending: false,
    error: planState.error,
  }),
  useApplyIac: () => ({
    mutate: vi.fn(),
    reset: vi.fn(),
    isPending: false,
    data: undefined,
    error: null,
  }),
}))
vi.mock('../IacExportCard', () => ({ IacExportCard: () => null }))
vi.mock('../IacPlanView', () => ({
  IacPlanView: () => null,
  IacResultView: () => null,
}))

describe('InfrastructurePage', () => {
  it('shows the hero banner with the upload action before any file is added', () => {
    planState.error = null
    render(<InfrastructurePage />)
    expect(screen.getByTestId('page-banner-full')).toBeTruthy()
    expect(
      screen.getAllByRole('button', { name: 'Upload files' }),
    ).toHaveLength(1)
  })

  it('goes compact once YAML is pasted', () => {
    planState.error = null
    render(<InfrastructurePage />)
    fireEvent.change(screen.getByLabelText('YAML'), {
      target: { value: 'version: 1' },
    })
    expect(screen.getByTestId('page-banner-compact')).toBeTruthy()
  })

  it('offers Plan again when planning fails', () => {
    planState.error = new Error('bad yaml')
    render(<InfrastructurePage />)
    fireEvent.change(screen.getByLabelText('YAML'), {
      target: { value: 'version: 1' },
    })
    fireEvent.click(screen.getByRole('button', { name: /Plan again/ }))
    expect(planMutate).toHaveBeenCalledTimes(1)
  })
})
