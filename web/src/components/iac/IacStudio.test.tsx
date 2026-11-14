import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import i18next from 'i18next'
import { initReactI18next } from 'react-i18next'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import commonEn from '../../locales/en/common.json'
import settingsEn from '../../locales/en/settings.json'
import iacFormEn from '../../locales/en/iacForm.json'
import { IacStudio } from './IacStudio'

void i18next.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['common', 'settings', 'iacForm'],
  defaultNS: 'common',
  resources: {
    en: { common: commonEn, settings: settingsEn, iacForm: iacFormEn },
  },
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

const FORM_YAML = 'version: 1\nkind: App\nmetadata:\n  name: web\n'

describe('IacStudio form mode', () => {
  beforeEach(() => {
    state.planError = null
    state.applyError = null
    localStorage.clear()
  })

  it('remembers the chosen view', async () => {
    const { unmount } = render(<IacStudio />)
    fireEvent.click(screen.getByRole('button', { name: 'Form' }))
    await screen.findByRole('form')
    unmount()
    render(<IacStudio />)
    expect(screen.getByRole('button', { name: 'Form' })).toHaveAttribute(
      'aria-pressed',
      'true',
    )
  })

  it('writes form edits back to the YAML', async () => {
    render(<IacStudio />)
    fireEvent.change(screen.getByLabelText('Resource YAML'), {
      target: { value: FORM_YAML },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Form' }))
    const name = await screen.findByRole('textbox', { name: /^Name/ })
    expect(name).toHaveValue('web')
    fireEvent.change(name, { target: { value: 'api' } })
    fireEvent.click(screen.getByRole('button', { name: 'YAML' }))
    expect(
      screen.getByLabelText<HTMLTextAreaElement>('Resource YAML').value,
    ).toContain('name: api')
  })

  it('shows YAML typed before switching to the form', async () => {
    render(<IacStudio />)
    fireEvent.change(screen.getByLabelText('Resource YAML'), {
      target: { value: FORM_YAML.replace('web', 'typed') },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Form' }))
    expect(await screen.findByDisplayValue('typed')).toBeInTheDocument()
  })

  it('locks the form and keeps the text for several documents', async () => {
    const multi = `${FORM_YAML}---\nversion: 1\nkind: App\nmetadata:\n  name: second\n`
    render(<IacStudio />)
    fireEvent.change(screen.getByLabelText('Resource YAML'), {
      target: { value: multi },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Form' }))
    await waitFor(() =>
      expect(screen.getByText(/more than one document/)).toBeInTheDocument(),
    )
    expect(screen.getByRole('textbox', { name: /^Name/ })).toBeDisabled()
    fireEvent.click(screen.getByRole('button', { name: 'YAML' }))
    expect(screen.getByLabelText('Resource YAML')).toHaveValue(multi)
  })

  it('offers edit as YAML on a parse error', async () => {
    render(<IacStudio />)
    fireEvent.change(screen.getByLabelText('Resource YAML'), {
      target: { value: 'kind: [App\n' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Form' }))
    fireEvent.click(await screen.findByRole('button', { name: 'Edit as YAML' }))
    expect(screen.getByLabelText('Resource YAML')).toBeInTheDocument()
  })
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
