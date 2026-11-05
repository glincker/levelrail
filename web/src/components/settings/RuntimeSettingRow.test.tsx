import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { AllI18n } from '../../test/allI18n'
import type { RuntimeSetting } from '../../types/runtimeSettings'
import { RuntimeSettingRow } from './RuntimeSettingRow'

const buildNetwork: RuntimeSetting = {
  key: 'security.build_network',
  category: 'security',
  kind: 'string',
  title: 'Build network',
  description: 'Network for RUN steps',
  default: 'open',
  value: 'open',
  source: 'default',
  env: 'APP_BUILD_NETWORK',
  apply: 'live',
  risk: 'weakens_security',
  onboarding: false,
  allowed_strings: ['open', 'none', 'block-private'],
  pending_restart: false,
}

describe('RuntimeSettingRow string kind', () => {
  it('renders a select of the allowed words, not a text area', () => {
    render(
      <QueryClientProvider client={new QueryClient()}>
        <AllI18n>
          <RuntimeSettingRow setting={buildNetwork} canEdit />
        </AllI18n>
      </QueryClientProvider>,
    )
    expect(screen.queryByRole('textbox')).toBeNull()
    fireEvent.click(screen.getByRole('combobox', { name: 'Build network' }))
    expect(screen.getAllByRole('option')).toHaveLength(3)
    expect(screen.getByRole('option', { name: /^None/ })).toBeTruthy()
  })
})

describe('RuntimeSettingRow weakening reset', () => {
  it('asks to confirm, then resets with confirm_weakening', async () => {
    const calls: string[] = []
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string, init?: RequestInit) => {
        calls.push(`${init?.method} ${url}`)
        if (!url.includes('confirm_weakening')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                error: 'resetting restores a weaker value',
                code: 'confirmation_required',
              }),
              { status: 400, headers: { 'Content-Type': 'application/json' } },
            ),
          )
        }
        return Promise.resolve(
          new Response(JSON.stringify(buildNetwork), { status: 200 }),
        )
      }),
    )
    render(
      <QueryClientProvider client={new QueryClient()}>
        <AllI18n>
          <RuntimeSettingRow
            setting={{ ...buildNetwork, value: 'none', source: 'saved' }}
            canEdit
          />
        </AllI18n>
      </QueryClientProvider>,
    )
    fireEvent.click(screen.getByRole('button', { name: /reset/i }))
    fireEvent.click(
      await screen.findByRole('button', { name: 'Lower protection' }),
    )
    await waitFor(() => expect(calls).toHaveLength(2))
    expect(calls[1]).toContain('DELETE')
    expect(calls[1]).toContain('?confirm_weakening=true')
    vi.unstubAllGlobals()
  })
})

describe('RuntimeSettingRow risk badge', () => {
  function renderRow(setting: RuntimeSetting) {
    render(
      <QueryClientProvider client={new QueryClient()}>
        <AllI18n>
          <RuntimeSettingRow setting={setting} canEdit />
        </AllI18n>
      </QueryClientProvider>,
    )
  }

  it('shows a neutral marker on a safe default', () => {
    renderRow(buildNetwork)
    expect(screen.queryByText('Weakens security')).toBeNull()
    expect(screen.getByText('Security setting')).toBeTruthy()
  })

  it('warns only when the value is weaker than the default', () => {
    renderRow({ ...buildNetwork, value: 'none', weaker_than_default: true })
    expect(screen.getByText('Weakens security')).toBeTruthy()
  })

  it('shows no security badge on a normal risk setting', () => {
    renderRow({ ...buildNetwork, risk: 'normal' })
    expect(screen.queryByText('Security setting')).toBeNull()
  })
})
