import { fireEvent, render, screen } from '@testing-library/react'
import i18next from 'i18next'
import { initReactI18next } from 'react-i18next'
import { describe, expect, it, vi } from 'vitest'
import commonEn from '../../locales/en/common.json'
import settingsEn from '../../locales/en/settings.json'
import { IacStudio } from './IacStudio'

void i18next.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['common', 'settings'],
  defaultNS: 'common',
  resources: { en: { common: commonEn, settings: settingsEn } },
  interpolation: { escapeValue: false },
})

const state = vi.hoisted(() => ({
  planError: null as Error | null,
  applyError: null as (Error & { code?: string }) | null,
}))

vi.mock('@tanstack/react-query', () => ({
  useQuery: () => ({ data: { LoadBalancer: 'beta' } }),
}))
vi.mock('./IacTemplateStrip', () => ({
  IacTemplateStrip: ({ onUse }: { onUse: (y: string) => void }) => (
    <button type="button" onClick={() => onUse('kind: App\n')}>
      use-template
    </button>
  ),
}))
vi.mock('../IacPlanView', () => ({
  IacPlanView: () => <div>plan-view</div>,
  IacResultView: () => null,
}))
vi.mock('../../queries/iac', () => {
  class IacIssuesError extends Error {
    issues = [
      {
        line: 2,
        path: 'spec.port',
        message: 'bad port',
        code: 'invalid_field',
      },
    ]
  }
  return {
    IacIssuesError,
    iacSchemaQueryOptions: () => ({}),
    usePlanIac: () => ({
      mutate: vi.fn(),
      isPending: false,
      error: state.planError ? new IacIssuesError() : null,
    }),
    useApplyIac: () => ({
      mutate: vi.fn(),
      reset: vi.fn(),
      isPending: false,
      data: undefined,
      error: state.applyError,
    }),
  }
})

describe('IacStudio', () => {
  it('fills the editor from a template', () => {
    state.planError = null
    state.applyError = null
    render(<IacStudio />)
    fireEvent.click(screen.getByText('use-template'))
    expect(screen.getByLabelText('Resource YAML')).toHaveValue('kind: App\n')
  })

  it('shows the code badge for a plan issue', async () => {
    state.planError = new Error('x')
    render(<IacStudio />)
    fireEvent.change(screen.getByLabelText('Resource YAML'), {
      target: { value: 'kind: Bad' },
    })
    await screen.findByText('invalid_field')
    expect(screen.getByText('invalid_field')).toBeInTheDocument()
    expect(screen.getByText(/bad port/)).toBeInTheDocument()
  })

  it('shows the kill switch banner on apply_disabled', () => {
    state.planError = null
    state.applyError = Object.assign(new Error('off'), {
      code: 'apply_disabled',
    })
    render(<IacStudio />)
    expect(screen.getByRole('alert')).toHaveTextContent(/switched off/)
  })
})
